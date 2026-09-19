import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { TrashTab } from './TrashTab';

const item = { id: 5, kind: 'file', workTitle: 'Duna', originalPath: 'Frank Herbert/Duna/Duna.epub', sizeBytes: 2048,
  trashedAt: '2026-09-01T10:00:00Z', purgeAfter: null };
let view;

async function open({ isOwner = true, items = [item], policy = { enabled: false, days: 30 } } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/trash') return { data: { items, totalBytes: 2048, policy } };
    if (url === '/admin/trash/policy/preview') return { data: { days: 10, items: 4, alreadyDue: 2 } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockResolvedValue({ data: {} });
  api.put.mockResolvedValue({});
  api.delete.mockResolvedValue({});
  view = await mount(<TrashTab isOwner={isOwner} />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('TrashTab', () => {
  it('shows what is in the trash and the space it takes', async () => {
    await open();
    expect(view.text()).toContain('Duna');
    expect(view.text()).toContain('2,0 KB');
  });

  it('restores an item without asking, since nothing is lost', async () => {
    await open();
    await view.click(view.button('Recuperar'));
    expect(api.post).toHaveBeenCalledWith('/admin/trash/5/restore');
  });

  it('deletes for good only after a confirmation that names the item', async () => {
    await open();
    await view.click(view.button('Apagar de vez'));
    expect(view.dialog().textContent).toContain('Duna');
    expect(api.delete).not.toHaveBeenCalled();

    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.delete).toHaveBeenCalledWith('/admin/trash/5', { params: { confirm: true } });
  });

  it('does not delete when the confirmation is cancelled', async () => {
    await open();
    await view.click(view.button('Apagar de vez'));
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
  });

  it('empties the trash only after confirming', async () => {
    await open();
    await view.click(view.button('Esvaziar'));
    expect(api.post).not.toHaveBeenCalled();
    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.post).toHaveBeenCalledWith('/admin/trash/empty', { confirm: true });
  });

  it('lets only the owner change the automatic cleanup', async () => {
    await open({ isOwner: false });
    expect(view.container.querySelector('input[type="checkbox"]')).toBeNull();
    view.unmount();

    await open({ isOwner: true });
    expect(view.container.querySelector('input[type="checkbox"]')).toBeTruthy();
  });

  it('previews the policy before applying it to the items already there', async () => {
    await open({ policy: { enabled: true, days: 10 } });
    await view.click(view.button('Ver o efeito nos itens atuais'));
    expect(view.text()).toContain('apagaria de vez 2');

    await view.click(view.button('Aplicar aos itens atuais'));
    expect(api.post).not.toHaveBeenCalled();
    await view.click(view.dialog().querySelectorAll('button')[1]);
    expect(api.post).toHaveBeenCalledWith('/admin/trash/policy/apply', { days: 10, confirm: true });
  });
});
