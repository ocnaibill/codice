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
  Element.prototype.scrollIntoView = vi.fn();
  api.get.mockReset().mockImplementation(async (url) => {
    if (url === '/auth/me')
      return { data: { id: 1, username: 'ana', role: 'reader' } };
    if (url === '/stats')
      return {
        data: {
          worksTotal: 25,
          libraryBreakdown: { livros: 20, mangas: 4, audio: 1 },
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
  it('paginates the catalog and resets to page one when its category changes', async () => {
    view = await mount(<HomePage />);
    await flush();
    await view.click(view.button('Próxima'));
    expect(api.get).toHaveBeenCalledWith('/works?page=2&limit=12');
    await view.click(view.buttonMatching(/^Mangás & HQs/));
    expect(api.get).toHaveBeenCalledWith(
      '/works?page=1&limit=12&formatGroup=comics'
    );
    expect(useGlobalStore.getState().libraryPage).toBe(1);
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
