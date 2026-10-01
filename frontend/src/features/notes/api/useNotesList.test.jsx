import { describe, it, expect, beforeEach, vi } from 'vitest';
import { afterEach } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { mount } from '../../admin/testUtils';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn() } }));

import { api } from '../../../lib/api';
import { filterParams, PAGE_SIZE, useNotesList } from './useNotesList';

beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ data: { data: [], total: 0 } });
});

describe('filterParams', () => {
  it('has only what was set, with the text trimmed and the work as a number', () => {
    expect(filterParams({}).toString()).toBe('');
    expect(filterParams({ q: '  areia ', kind: 'note', tag: 'ideia', workId: 7 }).toString()).toBe('q=areia&kind=note&tag=ideia&workId=7');
    expect(filterParams({ q: '   ', kind: '', tag: '', workId: null }).toString()).toBe('');
    expect(filterParams().toString()).toBe('');
  });

  it('encodes what a person typed', () => {
    expect(filterParams({ q: 'a&b=c' }).toString()).toBe('q=a%26b%3Dc');
  });
});

describe('useNotesList', () => {
  let view;
  const Probe = ({ filters, page }) => {
    const { data } = useNotesList(filters, page);
    return <p>{data ? `total ${data.total}` : 'loading'}</p>;
  };
  afterEach(() => view?.unmount());

  it('asks for one page at the offset that page starts at', async () => {
    view = await mount(<Probe filters={{ kind: 'highlight' }} page={1} />);
    expect(api.get).toHaveBeenLastCalledWith(`/notes?kind=highlight&limit=${PAGE_SIZE}&offset=0`);
    view.unmount();
    view = await mount(<Probe filters={{ kind: 'highlight' }} page={3} />);
    expect(api.get).toHaveBeenLastCalledWith(`/notes?kind=highlight&limit=${PAGE_SIZE}&offset=${2 * PAGE_SIZE}`);
    expect(view.text()).toContain('total 0');
  });

  it('keeps each page and filter under the notes key, so that saving a note anywhere refreshes the list', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { createRoot } = await import('react-dom/client');
    const { act } = await import('react');
    const container = document.createElement('div');
    const root = createRoot(container);
    await act(async () => { root.render(<QueryClientProvider client={client}><Probe filters={{}} page={1} /></QueryClientProvider>); });
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    expect(client.getQueryCache().getAll().map((q) => q.queryKey)).toEqual([['notes', 'list', { page: 1 }]]);
    act(() => root.unmount());
  });
});
