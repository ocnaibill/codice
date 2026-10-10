import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/download', () => ({ downloadFile: vi.fn(async () => {}) }));

import { useGlobalStore } from '../../../store/useGlobalStore';
import { NotesQuotes } from './NotesQuotes';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const note = (id, over = {}) => ({ id, quote: `Trecho ${id}`, workTitle: 'Duna', workAuthor: 'Frank Herbert', sourceAvailable: true, ...over });

let container;
let root;
const render = async (props) => { await act(async () => { root.render(<NotesQuotes {...props} />); }); };
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.startsWith(text));

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.setState({ notesOpen: false });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('NotesQuotes: what is in the margins, on the home', () => {
  it('shows the notes it was given, and a way to all of them when the person kept more', async () => {
    await render({ notes: [note(1), note(2)], total: 7, isLoading: false });
    expect(container.textContent).toContain('Trecho 1');
    expect(container.textContent).toContain('Trecho 2');
    expect(button('Ver todas (7)')).toBeTruthy();
  });

  it('takes the person to the page of the notes', async () => {
    await render({ notes: [note(1), note(2)], total: 7, isLoading: false });
    await act(async () => { button('Ver todas').click(); });
    expect(useGlobalStore.getState().notesOpen).toBe(true);
  });

  it('has no way to "all" when what is shown is all there is', async () => {
    await render({ notes: [note(1), note(2)], total: 2, isLoading: false });
    expect(button('Ver todas')).toBeUndefined();
    await render({ notes: [note(1)], total: 1, isLoading: false });
    expect(button('Ver todas')).toBeUndefined();
    await render({ notes: [note(1)], isLoading: false }); // a server that does not say how many
    expect(button('Ver todas')).toBeUndefined();
  });

  it('keeps the way to export all of them', async () => {
    await render({ notes: [note(1)], total: 1, isLoading: false });
    expect(button('Exportar tudo')).toBeTruthy();
  });

  it('says there is none when there is none, with no links', async () => {
    await render({ notes: [], total: 0, isLoading: false });
    expect(container.textContent).toContain('Nenhuma anotação ainda');
    expect(button('Ver todas')).toBeUndefined();
    expect(button('Exportar tudo')).toBeUndefined();
  });

  it('says a note with no passage by what was written, and where the source is gone', async () => {
    await render({ notes: [note(1, { quote: '', body: 'Uma ideia minha.', sourceAvailable: false })], total: 1, isLoading: false });
    expect(container.textContent).toContain('Uma ideia minha.');
    expect(container.textContent).toContain('(fonte indisponível)');
  });
});
