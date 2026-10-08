import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '../../lib/api';
import { mount } from '../../features/admin/testUtils';
import { Sidebar } from './Sidebar';
import { useGlobalStore } from '../../store/useGlobalStore';

let view;
const open = async ({ total = 2, canAdmin = false } = {}) => {
  api.get.mockImplementation(async (url, options) => {
    if (url === '/stats') return { data: { worksTotal: 7, libraryBreakdown: { livros: 7, mangas: 0, audio: 0 }, inProgressCount: 0 } };
    if (url === '/collections' && options.params.kind === 'personal') {
      expect(options.params).toEqual({ page: 1, limit: 1, kind: 'personal' });
      return { data: { data: [], total: 5 } };
    }
    if (url === '/collections') {
      expect(options.params).toEqual({ page: 1, limit: 1 });
      return { data: { data: [], total } };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  view = await mount(<Sidebar canAdmin={canAdmin} />);
};
const items = () => [...document.body.querySelectorAll('nav button')].map((b) => b.textContent.trim());
beforeEach(() => {
  vi.clearAllMocks();
  useGlobalStore.setState({ notesOpen: false, adminOpen: false, searchQuery: '', libraryView: 'all' });
});
afterEach(() => view.unmount());

describe('the menu of the library: collections', () => {
  it('offers them, with their count, in the library section', async () => {
    await open({ total: 2 });
    expect(items()).toEqual(['Todas as obras[07]', 'Livros digitais[07]', 'Coleções[02]', 'Em leitura[00]', 'Favoritos', 'Minhas listas[05]', 'Anotações']);
  });

  it('does not offer them to a reader while there is none', async () => {
    await open({ total: 0 });
    expect(items().join('|')).not.toContain('Coleções');
  });

  it('offers them to the staff even with none, so that the first is made', async () => {
    await open({ total: 0, canAdmin: true });
    expect(items()).toContain('Coleções[00]');
  });

  it('always offers the lists of the person, with their count, for a reader too', async () => {
    await open({ total: 0 });
    expect(items()).toContain('Minhas listas[05]');
    await view.click(view.buttonMatching(/^Minhas listas/));
    expect(useGlobalStore.getState().libraryView).toBe('lists');
    expect(view.buttonMatching(/^Minhas listas/).getAttribute('aria-current')).toBe('page');
  });

  it('opens them, and marks them as the page that is open', async () => {
    await open({ total: 2 });
    await view.click(view.buttonMatching(/^Coleções/));
    expect(useGlobalStore.getState().libraryView).toBe('collections');
    expect(view.buttonMatching(/^Coleções/).getAttribute('aria-current')).toBe('page');
    expect(view.buttonMatching(/^Todas as obras/).getAttribute('aria-current')).toBeNull();
  });
});
