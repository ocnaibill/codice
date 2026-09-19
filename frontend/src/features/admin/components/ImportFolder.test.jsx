import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { ImportFolder } from './ImportFolder';

const summary = { scanned: 3, enqueued: 2, duplicates: 1, originalsRemoved: 2, cleanupPending: 0, errors: 0 };
let view;

beforeEach(async () => {
  vi.clearAllMocks();
  api.post.mockResolvedValue({ data: summary });
  view = await mount(<ImportFolder />);
});
afterEach(() => view.unmount());

describe('ImportFolder', () => {
  it('asks whether to delete the originals before doing anything', async () => {
    await view.type(view.container.querySelector('input'), '/livros');
    await view.click(view.button('Importar'));

    expect(view.dialog()).toBeTruthy();
    expect(view.text()).toContain('Apagar os originais depois de copiar?');
    expect(api.post).not.toHaveBeenCalled();
  });

  it('keeps the originals when that is the answer', async () => {
    await view.type(view.container.querySelector('input'), '/livros');
    await view.click(view.button('Importar'));
    await view.click(view.button('Manter originais'));

    expect(api.post).toHaveBeenCalledTimes(1);
    expect(api.post.mock.calls[0][0]).toBe('/works/bulk-import');
    expect(api.post.mock.calls[0][1]).toEqual({ directory: '/livros', removeOriginals: false });
    expect(view.dialog()).toBeNull();
  });

  it('deletes the originals only when asked to', async () => {
    await view.type(view.container.querySelector('input'), '/livros');
    await view.click(view.button('Importar'));
    await view.click(view.button('Apagar originais'));

    expect(api.post.mock.calls[0][1]).toEqual({ directory: '/livros', removeOriginals: true });
    expect(view.text()).toContain('Originais apagados: 2');
  });

  it('does nothing when the question is cancelled', async () => {
    await view.click(view.button('Importar'));
    await view.click(view.button('Cancelar'));

    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
  });

  it('asks again on the next import: the answer is never remembered', async () => {
    await view.click(view.button('Importar'));
    await view.click(view.button('Apagar originais'));
    await view.click(view.button('Importar'));

    expect(view.dialog()).toBeTruthy();
    expect(api.post).toHaveBeenCalledTimes(1);
  });

  it('closes the question with Escape without importing', async () => {
    await view.click(view.button('Importar'));
    await view.click({ click: () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })) });

    expect(view.dialog()).toBeNull();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('says so when the server refuses the folder', async () => {
    api.post.mockRejectedValue({ response: { status: 403, data: 'Forbidden: directory is outside the allowed import roots\n' } });
    await view.type(view.container.querySelector('input'), '/etc');
    await view.click(view.button('Importar'));
    await view.click(view.button('Manter originais'));

    expect(view.container.querySelector('[role="alert"]').textContent).toContain('outside the allowed import roots');
  });
});
