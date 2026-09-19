import { describe, it, expect, vi } from 'vitest';
import { refreshLibrary } from '../refreshLibrary';

describe('refreshLibrary', () => {
  it('refreshes the grid and also the counters and favourites derived from it', () => {
    const client = { invalidateQueries: vi.fn() };
    refreshLibrary(client);
    expect(client.invalidateQueries.mock.calls.map(([arg]) => arg.queryKey[0]).sort())
      .toEqual(['favorites', 'stats', 'works']);
  });
});
