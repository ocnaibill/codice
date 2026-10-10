import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { FeaturedQuote, WorkHighlights, highlightsOf } from './WorkHighlights';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const quote = (n, extra = {}) => ({
  id: n, kind: 'highlight', quote: `Uma passagem do livro que merece ser lida de novo, a de número ${n}.`, body: '', tags: [],
  chapter: '', locator: { type: 'epub', href: 'c.xhtml' }, fileId: 10, ...extra,
});

let container;
let root;
const render = async (ui) => { await act(async () => { root.render(ui); }); };

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
});

describe('highlightsOf', () => {
  it('keeps the highlights and the notes that hold a passage, and leaves out the bookmarks and the empty ones', () => {
    const notes = [quote(1), quote(2, { kind: 'note' }), quote(3, { kind: 'bookmark' }), quote(4, { quote: '   ' }), quote(5, { quote: '' })];
    expect(highlightsOf(notes).map((n) => n.id)).toEqual([1, 2]);
    expect(highlightsOf(undefined)).toEqual([]);
  });
});

describe('WorkHighlights', () => {
  it('says each highlight with the chapter and the page of a PDF, its words and its tags', async () => {
    await render(
      <WorkHighlights
        onOpen={vi.fn()}
        notes={[
          quote(1, { chapter: 'Capítulo 3', tags: ['medo', 'duna'] }),
          quote(2, { kind: 'note', body: 'Minha nota sobre isso', locator: { type: 'pdf', page: 44 }, chapter: 'Parte II' }),
        ]}
      />
    );
    const cards = [...container.querySelectorAll('li')].map((li) => li.textContent);
    expect(cards[0]).toContain('número 1');
    expect(cards[0]).toContain('Capítulo 3');
    expect(cards[0]).toContain('#medo #duna');
    expect(cards[1]).toContain('Minha nota sobre isso');
    expect(cards[1]).toContain('Parte II · pág. 45');
    expect(container.textContent).toContain('2 anotações salvas');
  });

  it('counts one in the singular', async () => {
    await render(<WorkHighlights onOpen={vi.fn()} notes={[quote(1)]} />);
    expect(container.textContent).toContain('1 anotação salva');
  });

  it('shows six and the rest on a press', async () => {
    const notes = Array.from({ length: 8 }, (_, i) => quote(i + 1));
    await render(<WorkHighlights onOpen={vi.fn()} notes={notes} />);
    expect(container.querySelectorAll('li')).toHaveLength(6);
    const more = [...container.querySelectorAll('button')].find((b) => b.textContent === 'Mostrar todos (8)');
    expect(more.getAttribute('aria-expanded')).toBe('false');
    await act(async () => { more.click(); });
    expect(container.querySelectorAll('li')).toHaveLength(8);
    expect([...container.querySelectorAll('button')].some((b) => b.textContent === 'Mostrar menos')).toBe(true);
  });

  it('has no press for six or fewer', async () => {
    await render(<WorkHighlights onOpen={vi.fn()} notes={Array.from({ length: 6 }, (_, i) => quote(i + 1))} />);
    expect([...container.querySelectorAll('button')].some((b) => b.textContent.startsWith('Mostrar'))).toBe(false);
  });

  it('opens the reader at the place of the highlight that was pressed', async () => {
    const onOpen = vi.fn();
    const notes = [quote(1), quote(2)];
    await render(<WorkHighlights onOpen={onOpen} notes={notes} />);
    await act(async () => { container.querySelectorAll('li button')[1].click(); });
    expect(onOpen).toHaveBeenCalledWith(notes[1]);
  });

  it('says nothing when there is none', async () => {
    await render(<WorkHighlights onOpen={vi.fn()} notes={[quote(1, { kind: 'bookmark' })]} />);
    expect(container.querySelector('section')).toBeNull();
    await render(<WorkHighlights onOpen={vi.fn()} notes={undefined} />);
    expect(container.querySelector('section')).toBeNull();
  });
});

describe('FeaturedQuote', () => {
  it('puts one of the highlights of a length that reads well alone, with its chapter', async () => {
    vi.spyOn(Math, 'random').mockReturnValue(0.99);
    const notes = [
      quote(1, { quote: 'Curta.' }),
      quote(2, { chapter: 'Capítulo 1' }),
      quote(3, { chapter: 'Capítulo 2' }),
      quote(4, { quote: 'x'.repeat(401) }),
      quote(5, { kind: 'bookmark' }),
    ];
    await render(<FeaturedQuote notes={notes} workId={7} onOpen={vi.fn()} />);
    const text = container.textContent;
    expect(text).toContain('número 3'); // the last of the two candidates, with the chance at its highest
    expect(text).toContain('Capítulo 2');
    expect(text).not.toContain('Curta.');
    expect(text).not.toContain('x'.repeat(401));
  });

  it('is another one when the chance is another, and stays the same while the notes are the same', async () => {
    const spy = vi.spyOn(Math, 'random').mockReturnValue(0);
    const notes = [quote(2), quote(3)];
    await render(<FeaturedQuote notes={notes} workId={7} onOpen={vi.fn()} />);
    expect(container.textContent).toContain('número 2');
    spy.mockReturnValue(0.99);
    await render(<FeaturedQuote notes={[...notes]} workId={7} onOpen={vi.fn()} />); // drawn again by the page, not the quote
    expect(container.textContent).toContain('número 2');
  });

  it('opens the reader at its place', async () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const onOpen = vi.fn();
    const notes = [quote(2)];
    await render(<FeaturedQuote notes={notes} workId={7} onOpen={onOpen} />);
    await act(async () => { [...container.querySelectorAll('button')].find((b) => b.textContent === 'Abrir no livro').click(); });
    expect(onOpen).toHaveBeenCalledWith(notes[0]);
  });

  it('says nothing when no highlight is of a length to stand alone', async () => {
    await render(<FeaturedQuote notes={[quote(1, { quote: 'Curta.' }), quote(2, { kind: 'bookmark' })]} workId={7} onOpen={vi.fn()} />);
    expect(container.querySelector('figure')).toBeNull();
    await render(<FeaturedQuote notes={undefined} workId={7} onOpen={vi.fn()} />);
    expect(container.querySelector('figure')).toBeNull();
  });
});
