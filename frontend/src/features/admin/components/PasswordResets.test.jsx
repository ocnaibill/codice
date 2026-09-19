import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { PasswordResets } from './PasswordResets';

const row = (over) => ({ id: 'p1', username: 'ana', role: 'reader', state: 'requested', requestedAt: '2026-09-01T10:00:00Z', canDecide: true, ...over });
let view;

async function open(requests) {
  api.get.mockResolvedValue({ data: { data: requests } });
  api.post.mockResolvedValue({ data: { id: 'p1', username: 'ana', token: 'segredo-abc', expiresAt: '2026-09-01T11:00:00Z' } });
  view = await mount(<PasswordResets />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('PasswordResets', () => {
  it('lists the requests with their state', async () => {
    await open([row(), row({ id: 'p2', username: 'bob', state: 'used', canDecide: false, decidedBy: 'boss' })]);
    expect(view.text()).toContain('ana');
    expect(view.text()).toContain('Aguardando');
    expect(view.text()).toContain('Usado');
    expect(view.text()).toContain('decidido por boss');
  });

  it('warns about identity and asks before generating the link', async () => {
    await open([row()]);
    await view.click(view.button('Aprovar'));
    expect(view.dialog().textContent).toContain('ter certeza de quem pediu');
    expect(api.post).not.toHaveBeenCalled();

    await view.click(view.button('Gerar link'));
    expect(api.post).toHaveBeenCalledWith('/password-resets/p1/approve');
  });

  it('shows the link once, with a warning', async () => {
    await open([row()]);
    await view.click(view.button('Aprovar'));
    await view.click(view.button('Gerar link'));
    const box = view.container.querySelector('input[aria-label="Link de redefinição"]');
    expect(box.value).toBe(`${window.location.origin}/?reset=segredo-abc`);
    expect(view.text()).toContain('ele não aparece de novo');
  });

  it('does not generate a link when the question is cancelled', async () => {
    await open([row()]);
    await view.click(view.button('Aprovar'));
    await view.click(view.button('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
  });

  it('rejects with one click and offers actions only where the server allows', async () => {
    await open([row(), row({ id: 'p2', username: 'adm2', role: 'admin', canDecide: false })]);
    expect([...document.querySelectorAll('button')].filter((b) => b.textContent === 'Aprovar')).toHaveLength(1);
    await view.click(view.button('Recusar'));
    expect(api.post).toHaveBeenCalledWith('/password-resets/p1/reject');
  });
});
