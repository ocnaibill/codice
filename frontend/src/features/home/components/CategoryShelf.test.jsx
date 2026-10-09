import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn() },
  authenticatedUrl: (url) => `${url}?t=x`,
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { CategoryShelf } from './CategoryShelf';

let view;
const tree = [
  { id: 1, parentId: null, name: 'Ficção científica', works: 3, own: 3, covers: ['/c/1.jpg', '/c/2.jpg'] },
  { id: 2, parentId: null, name: 'Mangá', works: 7, own: 1, covers: ['/c/3.jpg', '/c/4.jpg', '/c/5.jpg'] },
  { id: 3, parentId: 2, name: 'Seinen', works: 6, own: 6, covers: ['/c/3.jpg'] },
  { id: 4, parentId: null, name: 'Vazia', works: 0, own: 0 },
  { id: 5, parentId: null, name: 'Aventura', works: 3, own: 3 },
  { id: 6, parentId: null, name: 'Um só', works: 1, own: 1, covers: ['/c/9.jpg'] },
];

async function open(data = tree) {
  api.get.mockResolvedValue({ data: { data } });
  view = await mount(<CategoryShelf />);
}
const cards = () => [...document.body.querySelectorAll('section[aria-label="Explorar por categoria"] button')];
afterEach(() => view?.unmount());
beforeEach(() => {
  vi.clearAllMocks();
  useGlobalStore.setState({ categoryPageId: null, libraryPage: 1 });
});

describe('CategoryShelf', () => {
  it('asks for the covers along with the categories', async () => {
    await open();
    expect(api.get).toHaveBeenCalledWith('/categories', { params: { covers: 1 } });
  });

  it('shows the categories at the top that have works, the ones with most works first and ties by name', async () => {
    await open();
    expect(cards().map((b) => b.querySelector('span span').textContent)).toEqual(['Mangá', 'Aventura', 'Ficção científica', 'Um só']);
    expect(view.text()).toContain('Explorar por categoria');
  });

  it('says how many works each has, in the singular for one', async () => {
    await open();
    expect(cards()[0].textContent).toContain('7 obras');
    expect(cards()[3].textContent).toContain('1 obra');
    expect(cards()[3].textContent).not.toContain('1 obras');
  });

  it('does not show the subcategories nor the categories with no works', async () => {
    await open();
    expect(view.text()).not.toContain('Seinen');
    expect(view.text()).not.toContain('Vazia');
  });

  it('stacks the covers it was given, up to the three it sends, and none for a category without them', async () => {
    await open();
    const covers = (card) => [...card.querySelectorAll('img')].map((i) => i.getAttribute('src'));
    expect(covers(cards()[0])).toEqual(['/c/3.jpg?t=x', '/c/4.jpg?t=x', '/c/5.jpg?t=x']);
    expect(covers(cards()[1])).toEqual([]);
    expect(covers(cards()[2])).toEqual(['/c/1.jpg?t=x', '/c/2.jpg?t=x']);
    expect(cards()[0].querySelector('img').getAttribute('alt')).toBe('');
  });

  it('has a plain tile, and not a stack, for a category without covers', async () => {
    await open();
    expect(cards()[1].querySelector('.bg-surface-alt')).not.toBeNull();
    expect(cards()[1].querySelector('.relative')).toBeNull();
    expect(cards()[0].querySelector('.relative')).not.toBeNull();
    expect(cards()[0].querySelector('.bg-surface-alt')).toBeNull();
  });

  it('stacks the first cover on top', async () => {
    await open();
    const z = [...cards()[0].querySelectorAll('img')].map((i) => Number(i.style.zIndex));
    expect(z).toEqual([3, 2, 1]);
    const left = [...cards()[0].querySelectorAll('img')].map((i) => i.style.left);
    expect(left).toEqual(['0px', '12px', '24px']);
  });

  it('opens the page of the category', async () => {
    await open();
    await view.click(cards()[0]);
    expect(useGlobalStore.getState().categoryPageId).toBe(2);
  });

  it('is not there until some category has works: before there are any, with only empty ones, and while loading', async () => {
    await open([]);
    expect(view.text()).toBe('');
    view.unmount();
    await open([{ id: 1, parentId: null, name: 'Vazia', works: 0, own: 0 }]);
    expect(view.text()).toBe('');
    view.unmount();
    api.get.mockReturnValue(new Promise(() => {}));
    view = await mount(<CategoryShelf />);
    expect(view.text()).toBe('');
  });

  it('is not there when the categories could not be loaded', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    view = await mount(<CategoryShelf />);
    expect(view.text()).toBe('');
  });
});
