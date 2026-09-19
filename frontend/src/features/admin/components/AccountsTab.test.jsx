import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { AccountsTab } from './AccountsTab';

const accounts = [
  { id: 'o1', username: 'boss', email: 'b@x', role: 'owner', blockedAt: null, canBlock: false, isSelf: true },
  { id: 'r1', username: 'ana', email: 'a@x', role: 'reader', blockedAt: null, canBlock: true, isSelf: false },
  { id: 'r2', username: 'bob', email: 'bo@x', role: 'reader', blockedAt: '2026-09-01T10:00:00Z', canBlock: true, isSelf: false },
  { id: 'a2', username: 'adm', email: 'ad@x', role: 'admin', blockedAt: null, canBlock: false, isSelf: false },
];
let view;

beforeEach(async () => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ data: { data: accounts } });
  api.post.mockResolvedValue({});
  view = await mount(<AccountsTab />);
});
afterEach(() => view.unmount());

describe('AccountsTab', () => {
  it('shows each account with its role and whether it is blocked', async () => {
    expect(view.text()).toContain('boss');
    expect(view.text()).toContain('(você)');
    expect(view.text()).toContain('Dono');
    expect(view.text()).toContain('Bloqueada');
  });

  it('offers block or unblock only where the server allows it', async () => {
    const blockButtons = [...document.querySelectorAll('button')].filter((b) => b.textContent === 'Bloquear');
    expect(blockButtons).toHaveLength(1); // only ana: the owner, an admin and yourself are out
    expect(view.button('Desbloquear')).toBeTruthy(); // bob
  });

  it('blocks only after confirming', async () => {
    await view.click(view.button('Bloquear'));
    expect(view.dialog().textContent).toContain('Bloquear ana?');
    expect(api.post).not.toHaveBeenCalled();

    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.post).toHaveBeenCalledWith('/users/r1/block');
  });

  it('does not block when the question is cancelled', async () => {
    await view.click(view.button('Bloquear'));
    await view.click(view.button('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
  });

  it('unblocks with one click, since nothing is lost', async () => {
    await view.click(view.button('Desbloquear'));
    expect(api.post).toHaveBeenCalledWith('/users/r2/unblock');
  });

  it('shows what the server said when it refuses', async () => {
    api.post.mockRejectedValue({ response: { status: 403, data: 'Forbidden\n' } });
    await view.click(view.button('Desbloquear'));
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Forbidden');
  });
});
