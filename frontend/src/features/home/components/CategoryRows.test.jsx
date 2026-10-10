import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { act } from 'react';
import { mount, flush } from '../../admin/testUtils';
import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { CategoryRows, MAX_ROWS } from './CategoryRows';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() }, authenticatedUrl: (u) => u }));

let view;
afterEach(() => {
  view?.unmount();
  vi.unstubAllGlobals();
});

const cat = (id, name, works, parentId = null) => ({ id, parentId, name, works, own: works });
const work = (id, title) => ({ id, title, author: 'A', authors: [], coverUrl: '/c.jpg', tags: [], format: 'epub', mediaStatus: 'READY', fileCount: 1 });

function serve(tree, perCategory = (id) => [work(id * 100, `Obra da ${id}`)]) {
  api.get.mockReset().mockImplementation(async (url, config) => {
    if (url === '/categories') return { data: { data: tree } };
    if (url.startsWith('/works?')) {
      const params = new URLSearchParams(url.split('?')[1]);
      return { data: { data: perCategory(Number(params.get('category'))), total: 1, totalPages: 1, page: 1 } };
    }
    throw new Error(`unexpected GET ${url}`);
  });
}
const headings = () => [...document.body.querySelectorAll('h2')].map((h) => h.textContent);
const worksAsked = () => api.get.mock.calls.filter(([url]) => url.startsWith('/works?')).map(([url]) => new URLSearchParams(url.split('?')[1]).get('category'));

beforeEach(() => useGlobalStore.setState({ categoryPageId: null }));

describe('CategoryRows', () => {
  it('has a shelf for each category at the top that has works, the ones with most works first, one under the other', async () => {
    serve([cat(1, 'Ficção científica', 3), cat(2, 'Mangá', 7), cat(3, 'Vazia', 0), cat(4, 'Sub', 9, 1), cat(5, 'Clássicos', 7)]);
    view = await mount(<CategoryRows />);
    await flush();
    expect(headings()).toEqual(['Clássicos', 'Mangá', 'Ficção científica']); // 7 and 7 by name, then 3; no empty one and no sub-category
    expect(document.body.textContent).toContain('Obra da 5');
  });

  it('says how many works a category has and that the shelf is of the series together', async () => {
    serve([cat(1, 'Ficção científica', 3)]);
    view = await mount(<CategoryRows />);
    await flush();
    expect(document.body.textContent).toContain('[ 3 obras ]');
    expect(api.get).toHaveBeenCalledWith('/works?page=1&limit=12&series=collapse&category=1');
  });

  it('opens the page of the category from its link', async () => {
    serve([cat(2, 'Mangá', 7)]);
    view = await mount(<CategoryRows />);
    await flush();
    await view.click(view.buttonMatching(/Ver categoria/));
    expect(useGlobalStore.getState().categoryPageId).toBe(2);
  });

  it('has none until there is a category with works', async () => {
    serve([cat(1, 'Vazia', 0), cat(2, 'Sub', 2, 1)]);
    view = await mount(<CategoryRows />);
    await flush();
    expect(headings()).toEqual([]);
    expect(worksAsked()).toEqual([]);
    expect(document.body.querySelector('[aria-label="Por categoria"]')).toBeNull(); // not even an empty place
  });

  it('is only as many shelves as it was told, and the works of the others are not asked for', async () => {
    serve(Array.from({ length: MAX_ROWS + 4 }, (_, i) => cat(i + 1, `Categoria ${i + 1}`, 100 - i)));
    view = await mount(<CategoryRows />);
    await flush();
    expect(headings()).toHaveLength(MAX_ROWS);
    expect(worksAsked()).toHaveLength(MAX_ROWS);
    expect(MAX_ROWS).toBe(6);
  });

  it('asks for the works of a shelf only when it comes near the screen', async () => {
    const observers = [];
    vi.stubGlobal('IntersectionObserver', class {
      constructor(callback, options) { this.callback = callback; this.options = options; observers.push(this); }
      observe(el) { this.el = el; }
      disconnect() { this.gone = true; }
    });
    serve([cat(1, 'A', 5), cat(2, 'B', 4), cat(3, 'C', 3)]);
    view = await mount(<CategoryRows />);
    await flush();
    expect(worksAsked()).toEqual([]);            // nobody is on view yet
    expect(headings()).toEqual([]);
    expect(observers).toHaveLength(3);
    expect(observers[0].options.rootMargin).toBe('300px 0px');
    await act(async () => observers[1].callback([{ isIntersecting: true }]));
    await flush();
    expect(worksAsked()).toEqual(['2']);          // only the one that came
    expect(headings()).toEqual(['B']);
    expect(observers[1].gone).toBe(true);
    await act(async () => observers[0].callback([{ isIntersecting: false }]));
    expect(worksAsked()).toEqual(['2']);          // one that is only passing by the edge is not on view
  });

  it('says it when the works of a shelf did not come, without losing the others', async () => {
    api.get.mockReset().mockImplementation(async (url) => {
      if (url === '/categories') return { data: { data: [cat(1, 'A', 5), cat(2, 'B', 4)] } };
      const category = new URLSearchParams(url.split('?')[1]).get('category');
      if (category === '1') return { data: { data: [], total: 0, totalPages: 1, page: 1 } };
      return { data: { data: [work(9, 'Obra da B')], total: 1, totalPages: 1, page: 1 } };
    });
    view = await mount(<CategoryRows />);
    await flush();
    expect(headings()).toEqual(['A', 'B']);
    expect(document.body.textContent).toContain('Nenhuma obra encontrada nessa categoria ainda.');
    expect(document.body.textContent).toContain('Obra da B');
  });
});
