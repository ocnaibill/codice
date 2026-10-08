import { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, flush } from '../features/admin/testUtils';
import { HomePage } from './HomePage';
import { AppShell } from '../components/layout/AppShell';
import { useGlobalStore } from '../store/useGlobalStore';
import { api } from '../lib/api';

vi.mock('../lib/api', () => ({
  api: { get: vi.fn() },
  authenticatedUrl: (url) => url,
}));
let view;
const work = {
  id: 1,
  title: 'Duna',
  author: 'Frank Herbert',
  format: 'epub',
  tags: [],
  mediaStatus: 'READY',
  fileCount: 1,
};
beforeEach(() => {
  useGlobalStore.getState().setLibraryView('all');
  useGlobalStore.getState().setLibraryViewMode('grid');
  useGlobalStore.getState().setLibrarySort('added');
  Element.prototype.scrollIntoView = vi.fn();
  api.get.mockReset().mockImplementation(async (url) => {
    if (url === '/auth/me')
      return { data: { id: 1, username: 'ana', role: 'reader' } };
    if (url === '/stats')
      return {
        data: {
          worksTotal: 25,
          libraryBreakdown: { livros: 20, quadrinhos: 4, mangas: 3, audio: 1 },
        },
      };
    if (url.startsWith('/works?')) {
      const params = new URLSearchParams(url.split('?')[1]);
      return {
        data: {
          data: [work],
          total: 25,
          totalPages: 3,
          page: Number(params.get('page')),
        },
      };
    }
    return { data: { data: [], total: 0 } };
  });
});
afterEach(() => view?.unmount());

describe('library hub', () => {
  it('sorts the catalog by the author when asked, from the first page, and keeps the sort across categories', async () => {
    view = await mount(<HomePage />);
    await flush();
    await view.click(view.button('Próxima'));
    const select = document.body.querySelector('select[aria-label="Ordenar o acervo"]');
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
      setter.call(select, 'author');
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
    expect(api.get).toHaveBeenCalledWith('/works?page=1&limit=12&sort=author');
    expect(view.text()).toContain('Todas as obras, por autor');
    expect(view.text()).not.toContain('Adicionados recentemente');
    expect(useGlobalStore.getState().librarySort).toBe('author');
    await view.click(view.buttonMatching(/^Quadrinhos/));
    expect(api.get).toHaveBeenCalledWith('/works?page=1&limit=12&formatGroup=comics&sort=author');
    expect(view.text()).toContain('Quadrinhos'); // a category is named by what it holds
  });

  it('shows the mangas apart from the comics, each shelf named by what it holds (#187)', async () => {
    view = await mount(<HomePage />);
    await flush();
    await view.click(view.buttonMatching(/^Mangás/));
    expect(api.get).toHaveBeenCalledWith('/works?page=1&limit=12&formatGroup=mangas');
    expect(useGlobalStore.getState().libraryView).toBe('mangas');
    const headings = () => [...view.container.querySelectorAll('h2')].map((h) => h.textContent);
    expect(headings()).toContain('Mangás');
    await view.click(view.buttonMatching(/^Quadrinhos/));
    expect(api.get).toHaveBeenCalledWith('/works?page=1&limit=12&formatGroup=comics');
    expect(headings()).toContain('Quadrinhos');
    expect(headings()).not.toContain('Mangás');
  });

  it('paginates the catalog and resets to page one when its category changes', async () => {
    view = await mount(<HomePage />);
    await flush();
    await view.click(view.button('Próxima'));
    expect(api.get).toHaveBeenCalledWith('/works?page=2&limit=12');
    await view.click(view.buttonMatching(/^Quadrinhos/));
    expect(api.get).toHaveBeenCalledWith(
      '/works?page=1&limit=12&formatGroup=comics'
    );
    expect(useGlobalStore.getState().libraryPage).toBe(1);
  });

  it('does not show cards from the previous filter while the API loads', async () => {
    let finishComics;
    const originalGet = api.get.getMockImplementation();
    api.get.mockImplementation((url) => {
      if (url.includes('formatGroup=comics')) {
        return new Promise((resolve) => { finishComics = resolve; });
      }
      return originalGet(url);
    });
    view = await mount(<HomePage />);
    await flush();
    expect(view.container.querySelector('.library-books').textContent).toContain('Duna');

    await act(async () => useGlobalStore.getState().setLibraryView('comics'));
    expect(view.container.querySelector('.library-books').textContent).not.toContain('Duna');

    await act(async () => finishComics({
      data: { data: [{ ...work, id: 2, title: 'Volume Um', format: 'cbz' }], total: 1, totalPages: 1, page: 1 },
    }));
    await flush();
    expect(view.container.querySelector('.library-books').textContent).toContain('Volume Um');
  });

  it('changes the rendered layout when list mode is selected', async () => {
    view = await mount(<HomePage />);
    await flush();
    await view.click(view.container.querySelector('[aria-label="Lista"]'));
    expect(view.container.querySelector('.library-books').dataset.view).toBe(
      'list'
    );
    expect(
      view.container
        .querySelector('[aria-label="Lista"]')
        .getAttribute('aria-pressed')
    ).toBe('true');
  });

  it('uses the server filters for personal collections', async () => {
    view = await mount(<HomePage />);
    await act(async () =>
      useGlobalStore.getState().setLibraryView('favorites')
    );
    await flush();
    expect(api.get).toHaveBeenCalledWith(
      '/works?page=1&limit=12&favorite=true'
    );
    await act(async () => useGlobalStore.getState().setLibraryView('reading'));
    await flush();
    expect(api.get).toHaveBeenCalledWith(
      '/works?page=1&limit=12&inProgress=true'
    );
  });

  it('reports a catalog error instead of pretending that the library is empty', async () => {
    api.get.mockImplementation(async (url) => {
      if (url.includes('limit=12')) throw new Error('offline');
      return { data: { data: [], total: 0 } };
    });
    view = await mount(<HomePage />);
    await flush();
    expect(view.text()).toContain('Não foi possível carregar o acervo.');
    expect(view.text()).not.toContain('Nenhuma obra encontrada');
    api.get.mockResolvedValue({
      data: { data: [work], total: 1, totalPages: 1 },
    });
    await view.click(view.button('Tentar novamente'));
    await flush();
    expect(
      view.container.querySelector('.library-books').textContent
    ).toContain('Duna');
  });

  it('returns from search to the selected collection and hides staff controls from readers', async () => {
    useGlobalStore.getState().setSearchQuery('Duna');
    view = await mount(
      <AppShell searchQuery="Duna" canAdmin={false}>
        <p>Conteúdo</p>
      </AppShell>
    );
    await view.click(
      view.container.querySelector('.library-sidebar button.library-nav-item')
    );
    expect(useGlobalStore.getState()).toMatchObject({
      searchQuery: '',
      libraryView: 'all',
      adminOpen: false,
    });
    expect(view.text()).not.toContain('Administração');
    expect(view.text()).not.toContain('Adicionar');
  });
});

describe('library hub: collections', () => {
  const collections = (total) => ({ data: { data: [{ id: 1, kind: 'official', name: 'Duna', workCount: 2, completedCount: 0, coverUrl: '/c.jpg' }], total, totalPages: 1, page: 1 } });
  const withCollections = (role) => api.get.mockImplementation(async (url) => {
    if (url === '/auth/me') return { data: { id: 1, username: 'ana', role } };
    if (url === '/stats') return { data: { worksTotal: 25, libraryBreakdown: { livros: 25, mangas: 0, audio: 0 } } };
    if (url === '/collections') return collections(1);
    if (url.startsWith('/works?')) return { data: { data: [work], total: 25, totalPages: 1, page: 1 } };
    return { data: { data: [], total: 0 } };
  });

  it('shows the collections instead of the works when that shelf is picked, with no sort to choose', async () => {
    withCollections('reader');
    view = await mount(<HomePage />);
    await flush();
    expect(document.body.querySelector('select[aria-label="Ordenar o acervo"]')).not.toBeNull();
    await view.click(view.buttonMatching(/^Coleções/));
    expect(useGlobalStore.getState().libraryView).toBe('collections');
    expect(view.text()).toContain('Coleção');
    expect(document.body.querySelector('[aria-label="Abrir a coleção Duna"]')).not.toBeNull();
    expect(document.body.querySelector('select[aria-label="Ordenar o acervo"]')).toBeNull();
    expect(view.text()).not.toContain('Adicionados recentemente');
    await view.click(view.buttonMatching(/^Todos/));
    expect(view.text()).toContain('Adicionados recentemente');
    expect(document.body.querySelector('[aria-label="Abrir a coleção Duna"]')).toBeNull();
  });

  it('offers the shelf to the staff when there is no collection yet, and not to a reader', async () => {
    const none = (role) => api.get.mockImplementation(async (url) => {
      if (url === '/auth/me') return { data: { id: 1, username: 'ana', role } };
      if (url === '/stats') return { data: { worksTotal: 25, libraryBreakdown: { livros: 25, mangas: 0, audio: 0 } } };
      if (url === '/collections') return { data: { data: [], total: 0, totalPages: 0 } };
      if (url.startsWith('/works?')) return { data: { data: [work], total: 25, totalPages: 1, page: 1 } };
      return { data: { data: [], total: 0 } };
    });
    none('reader');
    view = await mount(<HomePage />);
    await flush();
    expect(view.buttonMatching(/^Coleções/)).toBeUndefined();
    view.unmount();
    none('admin');
    view = await mount(<HomePage />);
    await flush();
    expect(view.buttonMatching(/^Coleções/)).toBeTruthy();
  });
});

describe('library hub: the lists of the person', () => {
  const serve = (role = 'reader') => api.get.mockImplementation(async (url, options) => {
    if (url === '/auth/me') return { data: { id: 1, username: 'ana', role } };
    if (url === '/stats') return { data: { worksTotal: 25, libraryBreakdown: { livros: 25, mangas: 0, audio: 0 } } };
    if (url === '/collections') {
      return options?.params?.kind === 'personal'
        ? { data: { data: [{ id: 3, kind: 'personal', name: 'Para ler', workCount: 1, completedCount: 0, coverUrl: '/c.jpg' }], total: 1, totalPages: 1, page: 1 } }
        : { data: { data: [], total: 0, totalPages: 0, page: 1 } };
    }
    if (url.startsWith('/works?')) return { data: { data: [work], total: 25, totalPages: 1, page: 1 } };
    return { data: { data: [], total: 0 } };
  });

  it('offers the shelf to a reader, and shows the lists of the caller, with no sort to choose', async () => {
    serve('reader');
    view = await mount(<HomePage />);
    await flush();
    expect(view.buttonMatching(/^Minhas listas/).textContent).toBe('Minhas listas[01]');
    await view.click(view.buttonMatching(/^Minhas listas/));
    expect(useGlobalStore.getState().libraryView).toBe('lists');
    expect(document.body.querySelector('[aria-label="Abrir a lista Para ler"]')).not.toBeNull();
    expect(document.body.querySelector('select[aria-label="Ordenar o acervo"]')).toBeNull();
    expect(view.text()).not.toContain('Adicionados recentemente');
    expect(api.get).toHaveBeenCalledWith('/collections', { params: { page: 1, limit: 24, kind: 'personal' } });
  });
});

