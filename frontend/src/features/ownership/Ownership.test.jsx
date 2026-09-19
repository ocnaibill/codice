import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount } from '../admin/testUtils';
import { OwnershipBanner } from './OwnershipBanner';
import { TransferOwnership } from '../admin/components/TransferOwnership';

let view;
beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

const answer = (transfer, accounts = []) => api.get.mockImplementation(async (url) => {
  if (url === '/ownership/transfer') return { data: transfer };
  if (url === '/users') return { data: { data: accounts } };
  throw new Error(`unexpected GET ${url}`);
});

describe('OwnershipBanner', () => {
  it('shows nothing when there is no notice and no offer', async () => {
    answer({ transfer: null });
    view = await mount(<OwnershipBanner me={{ notices: [] }} />);
    expect(view.container.textContent).toBe('');
  });

  it('shows a notice about a recovery on the server and dismisses it on request', async () => {
    answer({ transfer: null });
    api.post.mockResolvedValue({});
    view = await mount(<OwnershipBanner me={{ notices: [{ id: 7, kind: 'owner_recovery_reset', details: {} }] }} />);
    expect(view.text()).toContain('link de recuperação');
    await view.click(view.button('Entendi'));
    expect(api.post).toHaveBeenCalledWith('/auth/notices/7/ack');
  });

  it('tells about a transfer done on the server, with names', async () => {
    answer({ transfer: null });
    view = await mount(<OwnershipBanner me={{ notices: [{ id: 1, kind: 'owner_recovery_transfer', details: { from: 'boss', to: 'ana', formerRole: 'reader' } }] }} />);
    expect(view.text()).toContain('de boss para ana');
  });

  it('shows the offer to the target and changes nothing until the password is entered', async () => {
    answer({ transfer: { from: 'boss', to: 'ana', formerRole: 'admin', expiresAt: '2026-09-30T00:00:00Z' }, outgoing: false });
    api.post.mockResolvedValue({});
    view = await mount(<OwnershipBanner me={{ notices: [] }} />);
    expect(view.text()).toContain('boss');
    expect(view.text()).toContain('Nada muda até você aceitar');

    await view.click(view.button('Aceitar…'));
    expect(api.post).not.toHaveBeenCalled();
    await view.type(view.container.querySelector('input[type="password"]'), 'minha-senha');
    await view.click(view.button('Aceitar a titularidade'));
    expect(api.post).toHaveBeenCalledWith('/ownership/transfer/accept', { password: 'minha-senha' });
  });

  it('declines with one click', async () => {
    answer({ transfer: { from: 'boss', to: 'ana', formerRole: 'reader', expiresAt: '2026-09-30T00:00:00Z' }, outgoing: false });
    api.post.mockResolvedValue({});
    view = await mount(<OwnershipBanner me={{ notices: [] }} />);
    await view.click(view.button('Recusar'));
    expect(api.post).toHaveBeenCalledWith('/ownership/transfer/decline');
  });

  it('does not show the owner their own outgoing offer as if it were addressed to them', async () => {
    answer({ transfer: { from: 'boss', to: 'ana', formerRole: 'reader', expiresAt: '2026-09-30T00:00:00Z' }, outgoing: true });
    view = await mount(<OwnershipBanner me={{ notices: [] }} />);
    expect(view.container.textContent).toBe('');
  });
});

describe('TransferOwnership', () => {
  const accounts = [
    { id: 'o', username: 'boss', role: 'owner', blockedAt: null },
    { id: 'a', username: 'ana', role: 'reader', blockedAt: null },
    { id: 'b', username: 'bob', role: 'reader', blockedAt: '2026-09-01T00:00:00Z' },
    { id: 'm', username: 'adm', role: 'admin', blockedAt: null },
  ];
  const select = async (index, value) => {
    const el = view.container.querySelectorAll('select')[index];
    await view.click({ click: () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, value);
      el.dispatchEvent(new Event('change', { bubbles: true }));
    } });
  };

  it('offers only accounts that could receive it', async () => {
    answer({ transfer: null }, accounts);
    view = await mount(<TransferOwnership />);
    const names = [...view.container.querySelectorAll('select')[0].options].map((o) => o.textContent);
    expect(names).toEqual(['Escolha uma conta', 'ana', 'adm']); // not the owner, not a blocked account
  });

  it('has no default for what you become, and needs everything before it can go', async () => {
    answer({ transfer: null }, accounts);
    view = await mount(<TransferOwnership />);
    expect(view.container.querySelectorAll('select')[1].value).toBe('');
    const go = () => view.button('Oferecer a titularidade');
    expect(go().disabled).toBe(true);

    await select(0, 'a');
    await select(1, 'reader');
    expect(go().disabled).toBe(true); // still no password
    await view.type(view.container.querySelector('input[type="password"]'), 'minha-senha');
    expect(go().disabled).toBe(false);
  });

  it('asks before offering, then sends the choices and the password', async () => {
    answer({ transfer: null }, accounts);
    api.post.mockResolvedValue({ data: {} });
    view = await mount(<TransferOwnership />);
    await select(0, 'a');
    await select(1, 'admin');
    await view.type(view.container.querySelector('input[type="password"]'), 'minha-senha');
    await view.click(view.button('Oferecer a titularidade'));
    expect(view.dialog().textContent).toContain('Oferecer a titularidade a ana?');
    expect(api.post).not.toHaveBeenCalled();

    await view.click(view.button('Oferecer'));
    expect(api.post).toHaveBeenCalledWith('/ownership/transfer', { targetId: 'a', formerRole: 'admin', password: 'minha-senha' });
  });

  it('shows a pending offer and lets the owner withdraw it', async () => {
    answer({ transfer: { from: 'boss', to: 'ana', formerRole: 'reader', expiresAt: '2026-09-30T00:00:00Z' }, outgoing: true }, accounts);
    api.delete.mockResolvedValue({});
    view = await mount(<TransferOwnership />);
    expect(view.text()).toContain('Oferta feita a ana');
    expect(view.container.querySelector('select')).toBeNull();
    await view.click(view.button('Cancelar a oferta'));
    expect(api.delete).toHaveBeenCalledWith('/ownership/transfer');
  });

  it('shows the reason when the server refuses', async () => {
    answer({ transfer: null }, accounts);
    api.post.mockRejectedValue({ response: { status: 403, data: 'The password is not correct\n' } });
    view = await mount(<TransferOwnership />);
    await select(0, 'a');
    await select(1, 'reader');
    await view.type(view.container.querySelector('input[type="password"]'), 'errada');
    await view.click(view.button('Oferecer a titularidade'));
    await view.click(view.button('Oferecer'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('The password is not correct');
  });
});
