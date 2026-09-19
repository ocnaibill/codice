import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../../../lib/api';
import { EditBookModal } from './EditBookModal';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

// What the list card knows about a book: no ISBN, publisher, language or locks.
const card = { id: 7, title: 'Duna', author: 'Frank Herbert', tags: ['Sci-Fi'], format: 'epub' };

const fullRecord = {
  ...card,
  metadata: {
    series: 'Crônicas de Duna', seriesIndex: 1, isbn: '9788576573135', publisher: 'Aleph',
    language: 'pt', publicationDate: '2017', description: 'Sinopse original',
    locks: { title: true, publisher: false }, sources: {},
  },
};

let container;
let root;

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

async function render(candidates = []) {
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: fullRecord };
    if (url === '/works/7/candidates') return { data: { data: candidates } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({});
  api.post.mockResolvedValue({});
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>
        <EditBookModal book={card} onClose={() => {}} />
      </QueryClientProvider>
    );
  });
  await flush();
}

const inputByValue = (v) => [...container.querySelectorAll('input, textarea')].find((i) => i.value === v);
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('EditBookModal', () => {
  it('starts from the full record, not from the list card', async () => {
    await render();
    expect(inputByValue('9788576573135')).toBeTruthy();
    expect(inputByValue('Aleph')).toBeTruthy();
    expect(inputByValue('Sinopse original')).toBeTruthy();
    expect(inputByValue('Crônicas de Duna')).toBeTruthy();
    expect(container.querySelector('#lock-title').checked).toBe(true);
    expect(container.querySelector('#lock-publisher').checked).toBe(false);
  });

  it('saves without erasing the fields the card never had', async () => {
    await render();
    await act(async () => { button('Save Changes').click(); });
    await flush();

    expect(api.put).toHaveBeenCalledTimes(1);
    const [url, body] = api.put.mock.calls[0];
    expect(url).toBe('/works/7');
    expect(body).toMatchObject({
      title: 'Duna', author: 'Frank Herbert', series: 'Crônicas de Duna', series_index: 1,
      isbn: '9788576573135', publisher: 'Aleph', language: 'pt',
      publication_date: '2017', description: 'Sinopse original', tags: ['Sci-Fi'],
      title_lock: true, publisher_lock: false,
    });
  });

  it('does not let the form be saved before the record has loaded', async () => {
    let release;
    api.get.mockImplementation((url) => {
      if (url === '/works/7') return new Promise((res) => { release = () => res({ data: fullRecord }); });
      return Promise.resolve({ data: { data: [] } });
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => {
      root.render(<QueryClientProvider client={client}><EditBookModal book={card} onClose={() => {}} /></QueryClientProvider>);
    });
    expect(button('Save Changes').disabled).toBe(true);
    await act(async () => { release(); });
    await flush();
    expect(button('Save Changes').disabled).toBe(false);
  });

  it('lists suggestions and decides them through the API', async () => {
    await render([{ id: 3, field: 'isbn', value: '111', source: 'openlibrary', current: '', evidence: {} }]);
    expect(container.textContent).toContain('Suggestions (1)');
    expect(container.textContent).toContain('nothing changes until you accept');
    expect(api.put).not.toHaveBeenCalled();

    await act(async () => { button('Accept').click(); });
    await flush();
    expect(api.post).toHaveBeenCalledWith('/works/7/candidates/3/accept');
  });

  it('retires instead of deleting', async () => {
    await render();
    expect(button('Retire from Library')).toBeTruthy();
    expect(container.textContent).not.toContain('Delete Book');
  });
});
