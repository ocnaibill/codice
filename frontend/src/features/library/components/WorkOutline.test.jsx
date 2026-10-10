import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { WorkOutline } from './WorkOutline';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const entry = (title, over = {}) => ({ title, depth: 0, part: 'body', hasChildren: false, locator: { type: 'epub', href: `${title}.xhtml` }, percent: 10, ...over });
const outline = {
  fileId: 1,
  current: 3,
  chapters: [
    entry('Capa', { part: 'front', percent: 0 }),
    entry('Parte I', { hasChildren: true, percent: 5 }),
    entry('Capítulo 1', { depth: 1, percent: 5 }),
    entry('Capítulo 2', { depth: 1, percent: 30 }),
    entry('Capítulo 3', { depth: 1, percent: 60 }),
    entry('Parte II', { hasChildren: true, percent: 70 }),
    entry('Capítulo 4', { depth: 1, percent: 70.4 }),
    entry('Apêndice', { part: 'back', percent: 95 }),
  ],
};

let container;
let root;
const render = async (props) => {
  await act(async () => { root.render(<WorkOutline onOpen={vi.fn()} {...props} />); });
};
const rows = () => [...container.querySelectorAll('li button')];
const textOf = (row) => row.textContent.replace(/\s+/g, ' ').trim();

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('WorkOutline', () => {
  it('lists the story, without the cover and the appendix, with how many chapters and parts there are', async () => {
    await render({ outline });
    expect(rows().map((r) => r.querySelector('.truncate').textContent)).toEqual(['Parte I', 'Capítulo 1', 'Capítulo 2', 'Capítulo 3', 'Parte II', 'Capítulo 4']);
    expect(container.textContent).toContain('4 capítulos · 2 partes');
  });

  it('marks what is before the place, the place itself and what is after, in words as well as in signs', async () => {
    await render({ outline });
    const [, c1, c2, c3] = rows();
    expect(textOf(c1)).toContain('✓');
    expect(textOf(c1)).toContain('Antes do ponto em que você está');
    expect(textOf(c2)).toContain('▶');
    expect(textOf(c2)).toContain('atual');
    expect(c2.getAttribute('aria-current')).toBe('location');
    expect(textOf(c3)).toContain('○');
    expect(textOf(c3)).toContain('Depois do ponto em que você está');
    expect(container.textContent).toContain('não o que foi lido');
  });

  it('marks nothing when the person has no place, and says no more than what it knows', async () => {
    await render({ outline: { ...outline, current: null } });
    expect(rows().every((r) => !textOf(r).includes('✓') && !textOf(r).includes('▶'))).toBe(true);
    expect(container.textContent).not.toContain('não o que foi lido');
  });

  it('says how far through the text each starts, and the page of a PDF', async () => {
    await render({ outline });
    expect(textOf(rows()[3])).toContain('60%');
    expect(textOf(rows()[5])).toContain('70%');
    const pdf = { current: null, chapters: [entry('Introdução', { locator: { type: 'pdf', page: 11 }, percent: 3 })] };
    act(() => root.unmount());
    root = createRoot(container);
    await render({ outline: pdf });
    expect(textOf(rows()[0])).toContain('pág. 12');
  });

  it('opens the reader at the entry, with its title for when the place cannot be opened', async () => {
    const onOpen = vi.fn();
    await render({ outline, onOpen });
    await act(async () => { rows()[3].click(); });
    expect(onOpen).toHaveBeenCalledWith({ type: 'epub', href: 'Capítulo 3.xhtml' }, 'Capítulo 3');
  });

  it('does not open an entry that holds no text', async () => {
    const onOpen = vi.fn();
    await render({ outline: { ...outline, chapters: [entry('Vazio', { locator: null })] }, onOpen });
    expect(rows()[0].disabled).toBe(true);
    await act(async () => { rows()[0].click(); });
    expect(onOpen).not.toHaveBeenCalled();
  });

  it('lists everything when the file has no entry that is the story', async () => {
    await render({ outline: { current: null, chapters: [entry('Nota', { part: 'front' }), entry('Índice', { part: 'back' })] } });
    expect(rows()).toHaveLength(2);
  });

  it('holds the hidden texts of its rows inside the list: they are absolute, and would be left under the page', async () => {
    await render({ outline });
    expect(container.querySelector('ol').className).toContain('relative');
  });

  it('says nothing when there is no outline', async () => {
    await render({ outline: { chapters: [], current: null } });
    expect(container.querySelector('section')).toBeNull();
    act(() => root.unmount());
    root = createRoot(container);
    await render({ outline: undefined });
    expect(container.querySelector('section')).toBeNull();
  });

  it('counts one chapter and no parts in the singular', async () => {
    await render({ outline: { current: null, chapters: [entry('Único')] } });
    expect(container.textContent).toContain('1 capítulo');
    expect(container.textContent).not.toContain('parte');
  });
});
