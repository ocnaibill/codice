import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { LibraryFilterBar } from './LibraryFilterBar';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let container;
let root;
beforeEach(() => { container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container); });
afterEach(() => { act(() => root.unmount()); container.remove(); });

const render = (props) => act(async () => { root.render(<LibraryFilterBar worksTotal={3} activeFilter="all" onFilterChange={() => {}} viewMode="grid" onViewModeChange={() => {}} {...props} />); });

describe('LibraryFilterBar: sorting', () => {
  it('offers newest first, title and author, with the one in use selected', async () => {
    await render({ sort: 'author', onSortChange: vi.fn() });
    const select = container.querySelector('select[aria-label="Ordenar o acervo"]');
    expect([...select.options].map((o) => [o.value, o.textContent])).toEqual([['added', 'Mais recentes'], ['title', 'Título'], ['author', 'Autor']]);
    expect(select.value).toBe('author');
  });

  it('tells which one was picked', async () => {
    const onSortChange = vi.fn();
    await render({ sort: 'added', onSortChange });
    const select = container.querySelector('select');
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
      setter.call(select, 'title');
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(onSortChange).toHaveBeenCalledWith('title');
  });

  it('has no sort control when nobody listens to it', async () => {
    await render({});
    expect(container.querySelector('select')).toBeNull();
  });
});
