import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { Invitations } from './Invitations';

const row = (over) => ({ id: 'i1', role: 'reader', state: 'pending', createdBy: 'boss', createdAt: '2026-09-01T10:00:00Z',
  expiresAt: '2026-09-08T10:00:00Z', canRevoke: true, ...over });
let view;

async function open({ isOwner = true, invitations = [] } = {}) {
  api.get.mockResolvedValue({ data: { data: invitations } });
  api.post.mockResolvedValue({ data: { id: 'n1', token: 'segredo-123', role: 'reader', email: '', expiresAt: '2026-09-08T10:00:00Z' } });
  api.delete.mockResolvedValue({});
  view = await mount(<Invitations isOwner={isOwner} />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('Invitations', () => {
  it('creates an invitation and shows the link once, with a warning', async () => {
    await open();
    await view.click(view.button('Criar convite'));

    expect(api.post).toHaveBeenCalledWith('/invitations', { role: 'reader', email: '' });
    const link = view.container.querySelector('input[aria-label="Link do convite"]');
    expect(link.value).toBe(`${window.location.origin}/?invite=segredo-123`);
    expect(view.text()).toContain('ele não aparece de novo');
  });

  it('sends the address when one is given', async () => {
    await open();
    await view.type(view.container.querySelector('input[type="email"]'), 'ana@exemplo.com');
    await view.click(view.button('Criar convite'));
    expect(api.post).toHaveBeenCalledWith('/invitations', { role: 'reader', email: 'ana@exemplo.com' });
  });

  it('lets only the owner choose the administrator role', async () => {
    await open({ isOwner: false });
    expect(view.container.querySelector('select')).toBeNull();
    view.unmount();

    await open({ isOwner: true });
    expect(view.container.querySelector('select')).toBeTruthy();
  });

  it('lists invitations with their state and never a secret', async () => {
    await open({ invitations: [row(), row({ id: 'i2', state: 'used', usedBy: 'ana', canRevoke: false })] });
    expect(view.text()).toContain('Pendente');
    expect(view.text()).toContain('usado por ana');
    expect(view.text()).not.toContain('segredo');
  });

  it('revokes only what the server says can be revoked', async () => {
    await open({ invitations: [row(), row({ id: 'i2', role: 'admin', canRevoke: false })] });
    const revoke = [...document.querySelectorAll('button')].filter((b) => b.textContent === 'Revogar');
    expect(revoke).toHaveLength(1);
    await view.click(revoke[0]);
    expect(api.delete).toHaveBeenCalledWith('/invitations/i1');
  });

  it('shows the reason when the server refuses', async () => {
    await open();
    api.post.mockRejectedValue({ response: { status: 403, data: 'Forbidden\n' } });
    await view.click(view.button('Criar convite'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Forbidden');
  });
});
