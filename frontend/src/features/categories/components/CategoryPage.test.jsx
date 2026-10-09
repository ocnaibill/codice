import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn() },
  authenticatedUrl: (url) => url,
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { CategoryPage } from './CategoryPage';

let view;
const tree = [
  { id: 1, parentId: null, name: 'Ficção científica', works: 3, own: 3 },
  { id: 2, parentId: null, name: 'Mangá', works: 7, own: 1 },
  { id: 3, parentId: 2, name: 'Seinen', works: 6, own: 1 },
  { id: 4, parentId: 2, name: 'Shounen', works: 0, own: 0 },
  { id: 5, parentId: 3, name: 'Dark', works: 5, own: 5 },
];
const work = (id, title) => ({ id, title, author: 'Alguém', format: 'cbz', tags: [], mediaStatus: 'READY', fileCount: 1, coverUrl: '' });
let worksAnswer;

async function open(id, { categories = tree, answer } = {}) {
  worksAnswer = answer ?? { data: [work(1, 'Berserk'), work(2, 'Vagabond')], total: 14, totalPages: 2, page: 1 };
  api.get.mockImplementation(async (url) => {
    if (url === '/categories') return { data: { data: categories } };
    if (url.startsWith('/works?')) return { data: worksAnswer };
    throw new Error(`unexpected GET ${url}`);
  });
  view = await mount(<CategoryPage id={id} />);
}
const worksCalls = () => api.get.mock.calls.map(([url]) => url).filter((url) => url.startsWith('/works?'));
afterEach(() => view?.unmount());
beforeEach(() => {
  vi.clearAllMocks();
  useGlobalStore.setState({ categoryPageId: null, libraryPage: 1, libraryView: 'all', libraryViewMode: 'grid', sheetWorkId: null });
});

describe('CategoryPage', () => {
  it('shows the page from the top when it opens, and again when another category opens', async () => {
    const scroll = vi.fn();
    Element.prototype.scrollIntoView = scroll;
    function Host() {
      const id = useGlobalStore((state) => state.categoryPageId);
      return <CategoryPage id={id} />;
    }
    try {
      useGlobalStore.setState({ categoryPageId: 2 });
      worksAnswer = { data: [work(1, 'Berserk')], total: 1, totalPages: 1, page: 1 };
      api.get.mockImplementation(async (url) => (url === '/categories' ? { data: { data: tree } } : { data: worksAnswer }));
      view = await mount(<Host />);
      expect(scroll).toHaveBeenCalledTimes(1);
      expect(scroll).toHaveBeenCalledWith({ block: 'start' });
      await view.click([...document.body.querySelectorAll('section[aria-label="Subcategorias"] button')][0]);
      expect(useGlobalStore.getState().categoryPageId).toBe(3);
      expect(scroll).toHaveBeenCalledTimes(2);
    } finally {
      delete Element.prototype.scrollIntoView;
    }
  });

  it('does not scroll again when only the works change', async () => {
    const scroll = vi.fn();
    Element.prototype.scrollIntoView = scroll;
    try {
      await open(2);
      await view.click(view.button('Próxima'));
      expect(scroll).toHaveBeenCalledTimes(1);
    } finally {
      delete Element.prototype.scrollIntoView;
    }
  });

  it('lists the works of the category by title, a page of 12, and says how many there are', async () => {
    await open(2);
    expect(worksCalls()).toEqual(['/works?page=1&limit=12&sort=title&category=2']);
    expect(view.text()).toContain('Berserk');
    expect(view.text()).toContain('Vagabond');
    expect(view.text()).toContain('[ 14 obras ]');
    expect(document.body.querySelector('h2').textContent).toBe('Mangá');
  });

  it('shows the subcategories that have works, each with how many, and not the ones with none', async () => {
    await open(2);
    const chips = [...document.body.querySelectorAll('section[aria-label="Subcategorias"] button')].map((b) => b.textContent);
    expect(chips).toEqual(['Seinen · 6']);
  });

  it('has no row of subcategories for one with none', async () => {
    await open(1);
    expect(document.body.querySelector('section[aria-label="Subcategorias"]')).toBeNull();
  });

  it('opens a subcategory from the first page', async () => {
    useGlobalStore.setState({ libraryPage: 2 });
    await open(2);
    const chip = [...document.body.querySelectorAll('section[aria-label="Subcategorias"] button')][0];
    await view.click(chip);
    expect(useGlobalStore.getState()).toMatchObject({ categoryPageId: 3, libraryPage: 1 });
  });

  it('says where the category is, with a way to each one above it and back to the library', async () => {
    await open(5);
    const trail = [...document.body.querySelectorAll('nav[aria-label="Onde você está"] button')].map((b) => b.textContent);
    expect(trail).toEqual(['← Acervo', 'Mangá', 'Seinen']);
    await view.click([...document.body.querySelectorAll('nav[aria-label="Onde você está"] button')][1]);
    expect(useGlobalStore.getState().categoryPageId).toBe(2);
  });

  it('has only the way back for a category at the top', async () => {
    await open(1);
    expect([...document.body.querySelectorAll('nav[aria-label="Onde você está"] button')].map((b) => b.textContent)).toEqual(['← Acervo']);
  });

  it('goes back to the library with the way back', async () => {
    useGlobalStore.setState({ categoryPageId: 2, libraryView: 'ebooks' });
    await open(2);
    await view.click(view.button('← Acervo'));
    expect(useGlobalStore.getState()).toMatchObject({ categoryPageId: null, libraryView: 'all' });
  });

  it('turns the pages of the works, from the page the store says', async () => {
    useGlobalStore.setState({ libraryPage: 2 });
    await open(2, { answer: { data: [work(3, 'Naruto')], total: 14, totalPages: 2, page: 2 } });
    expect(worksCalls()).toEqual(['/works?page=2&limit=12&sort=title&category=2']);
    expect(view.text()).toContain('2 de 2');
    expect(view.button('Próxima').disabled).toBe(true);
    await view.click(view.button('Anterior'));
    expect(useGlobalStore.getState().libraryPage).toBe(1);
  });

  it('has no pages when the works fit in one', async () => {
    await open(2, { answer: { data: [work(1, 'Berserk')], total: 1, totalPages: 1, page: 1 } });
    expect(view.button('Próxima')).toBeUndefined();
  });

  it('says there are no works in it when there are none', async () => {
    await open(1, { answer: { data: [], total: 0, totalPages: 0, page: 1 } });
    expect(view.text()).toContain('Nenhuma obra encontrada nessa categoria ainda.');
  });

  it('opens the sheet of a work', async () => {
    await open(2);
    await view.click([...document.body.querySelectorAll('button')].find((b) => b.textContent.includes('Berserk')));
    expect(useGlobalStore.getState().sheetWorkId).toBe(1);
  });

  it('says the category is gone when it is not in the tree any more', async () => {
    await open(99);
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('Esta categoria não existe mais.');
    expect(view.button('← Acervo')).toBeTruthy();
  });

  it('does not say the category is gone while the tree is still loading, and shows the works as loading', async () => {
    api.get.mockImplementation((url) => (url === '/categories' ? new Promise(() => {}) : Promise.resolve({ data: { data: [work(1, 'Berserk')], total: 1, totalPages: 1, page: 1 } })));
    view = await mount(<CategoryPage id={2} />);
    expect(view.container.querySelector('[role="alert"]')).toBeNull();
    expect(view.text()).not.toContain('não existe mais');
    expect(document.body.querySelector('section[aria-label="Obras da categoria"] section').getAttribute('aria-busy')).toBe('true');
    expect(view.text()).not.toContain('Berserk');
  });

  it('has no previous page on the first one', async () => {
    await open(2);
    expect(view.button('Anterior').disabled).toBe(true);
    expect(view.button('Próxima').disabled).toBe(false);
  });

  it('says it could not load the category, and asks again', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    view = await mount(<CategoryPage id={2} />);
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('Não foi possível carregar a categoria.');
    expect(view.button('Tentar novamente')).toBeTruthy();
  });

  it('says it could not load the works, and asks again, while the page still shows where it is', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/categories') return { data: { data: tree } };
      throw { response: { status: 500 } };
    });
    view = await mount(<CategoryPage id={2} />);
    expect(view.text()).toContain('Não foi possível carregar as obras desta categoria.');
    expect(view.text()).toContain('Seinen');
    api.get.mockClear();
    await view.click(view.button('Tentar novamente'));
    expect(worksCalls().length).toBeGreaterThan(0);
  });
});
