/**
 * Refreshes everything on screen that is derived from the catalog. The home page
 * shows counters, favourites and the grid from separate queries, so refreshing
 * only the grid leaves the numbers stale until the page is reloaded.
 */
export function refreshLibrary(queryClient) {
  for (const key of ['works', 'stats', 'favorites']) {
    queryClient.invalidateQueries({ queryKey: [key] });
  }
}
