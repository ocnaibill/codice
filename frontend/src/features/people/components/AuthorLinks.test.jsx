import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { AuthorLinks } from './AuthorLinks';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const render = (props) => act(async () => { root.render(<p><AuthorLinks {...props} /></p>); });

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.setState({ personSheetId: null, sheetWorkId: null });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('AuthorLinks: the authors of a work, each opening the page of the person (#186)', () => {
  it('shows each author as a button, with a comma between them', async () => {
    await render({ authors: [{ id: 3, name: 'Herbert, Frank' }, { id: 4, name: 'Brian Herbert' }], fallback: 'Frank Herbert, Brian Herbert' });
    expect([...container.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Herbert, Frank', 'Brian Herbert']);
    expect(container.querySelector('p').textContent).toBe('Herbert, Frank, Brian Herbert');
  });

  it('opens the page of the one that is pressed', async () => {
    await render({ authors: [{ id: 3, name: 'A' }, { id: 4, name: 'B' }] });
    await act(async () => { container.querySelectorAll('button')[1].click(); });
    expect(useGlobalStore.getState().personSheetId).toBe(4);
  });

  it('closes the sheet of the work it was pressed on, since the page takes its place', async () => {
    useGlobalStore.setState({ sheetWorkId: 7 });
    await render({ authors: [{ id: 3, name: 'A' }] });
    await act(async () => { container.querySelector('button').click(); });
    expect(useGlobalStore.getState()).toMatchObject({ personSheetId: 3, sheetWorkId: null });
  });

  it('says the text as it was when there are no authors one by one', async () => {
    await render({ authors: [], fallback: 'Unknown Author' });
    expect(container.querySelector('p').textContent).toBe('Unknown Author');
    expect(container.querySelector('button')).toBeNull();
    await render({ authors: undefined, fallback: 'Alguém' });
    expect(container.querySelector('p').textContent).toBe('Alguém');
    await render({});
    expect(container.querySelector('p').textContent).toBe('');
  });

  it('has a title that says what the name does, for those who cannot see the underline', async () => {
    await render({ authors: [{ id: 3, name: 'A' }] });
    expect(container.querySelector('button').title).toBe('Ver as obras desta pessoa');
  });
});
