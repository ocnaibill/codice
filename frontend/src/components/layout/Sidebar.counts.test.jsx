import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '../../lib/api';
import { mount } from '../../features/admin/testUtils';
import { Sidebar } from './Sidebar';
import { useGlobalStore } from '../../store/useGlobalStore';

let view;
const stats = (over = {}) => ({ worksTotal: 7, libraryBreakdown: { livros: 5, mangas: 0, audio: 2 }, inProgressCount: 3, ...over });
const open = async (data) => {
  api.get.mockImplementation(async (url) => {
    if (url !== '/stats') throw new Error(`unexpected GET ${url}`);
    return { data };
  });
  view = await mount(<Sidebar />);
};
const items = () => [...document.body.querySelectorAll('nav button')].map((b) => b.textContent.trim());
beforeEach(() => {
  vi.clearAllMocks();
  useGlobalStore.setState({ notesOpen: false, adminOpen: false, searchQuery: '', libraryView: 'all' });
});
afterEach(() => view.unmount());

describe('the menu of the library: the count of each shelf, and only the shelves that have something', () => {
  it('counts each shelf instead of naming the format', async () => {
    await open(stats({ libraryBreakdown: { livros: 5, mangas: 12, audio: 1420 } }));
    expect(items()).toEqual(['Todas as obras[07]', 'Livros digitais[05]', 'Quadrinhos & mangás[12]', 'Audiolivros[1.420]', 'Em leitura[03]', 'Favoritos', 'Anotações']);
    expect(document.body.textContent).not.toMatch(/EPUB|CBZ|CBR|PDF/);
  });

  it('offers a kind of work only when the library has at least one of it', async () => {
    await open(stats());
    // Comics: none, so no shelf of them. Audiobooks: two, so there is.
    expect(items()).toEqual(['Todas as obras[07]', 'Livros digitais[05]', 'Audiolivros[02]', 'Em leitura[03]', 'Favoritos', 'Anotações']);
  });

  it('offers no shelf of audiobooks to a library that has none, and keeps "Todas as obras" even when it is empty', async () => {
    await open(stats({ worksTotal: 0, libraryBreakdown: { livros: 0, mangas: 0, audio: 0 }, inProgressCount: 0 }));
    expect(items()).toEqual(['Todas as obras[00]', 'Em leitura[00]', 'Favoritos', 'Anotações']);
  });

  it('offers no kind while the counts are not known, and none if they cannot be read', async () => {
    api.get.mockReturnValue(new Promise(() => {}));
    view = await mount(<Sidebar />);
    expect(items()).toEqual(['Todas as obras', 'Em leitura', 'Favoritos', 'Anotações']);
    view.unmount();
    api.get.mockRejectedValue(new Error('offline'));
    view = await mount(<Sidebar />);
    expect(items()).toEqual(['Todas as obras', 'Em leitura', 'Favoritos', 'Anotações']);
  });

  it('shows the total of the library in the heading of the section', async () => {
    await open(stats({ worksTotal: 1420 }));
    expect(document.body.querySelector('.library-eyebrow').textContent).toBe('Biblioteca [1.420]');
  });

  it('marks the shelf that is open, and picking one opens it', async () => {
    useGlobalStore.setState({ libraryView: 'audio' });
    await open(stats());
    expect([...document.body.querySelectorAll('[aria-current="page"]')].map((b) => b.textContent.trim())).toEqual(['Audiolivros[02]']);
    await view.click(view.buttonMatching(/^Livros digitais/));
    expect(useGlobalStore.getState().libraryView).toBe('ebooks');
  });
});
