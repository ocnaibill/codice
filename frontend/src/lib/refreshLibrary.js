/**
 * Refreshes everything on screen that is derived from the catalog. The home page
 * shows counters, favourites and the grid from separate queries, so refreshing
 * only the grid leaves the numbers stale until the page is reloaded.
 */
export function refreshLibrary(queryClient) {
  // 'work' is the detail of one work, with the reader's position in each of its files.
  for (const key of ['works', 'work', 'stats', 'favorites']) {
    queryClient.invalidateQueries({ queryKey: [key] });
  }
}
