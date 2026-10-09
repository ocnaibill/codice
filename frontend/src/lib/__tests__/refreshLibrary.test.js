import { describe, it, expect, vi } from 'vitest';
import { refreshLibrary } from '../refreshLibrary';

describe('refreshLibrary', () => {
  it("refreshes the grid, the counters, favourites and collections derived from it, and the detail of a work (each file's position)", () => {
    const client = { invalidateQueries: vi.fn() };
    refreshLibrary(client);
    expect(client.invalidateQueries.mock.calls.map(([arg]) => arg.queryKey[0]).sort())
      .toEqual(['categories', 'collection', 'collections', 'favorites', 'person', 'stats', 'work', 'works']);
  });
});
