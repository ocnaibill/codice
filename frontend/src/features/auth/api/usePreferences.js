import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** How names of people are shown, and what applies to this account (#64, DEC-094). */
export const NAME_ORDERS = {
  given_first: { label: 'Nome Sobrenome', example: 'Frank Herbert' },
  family_first: { label: 'Sobrenome, Nome', example: 'Herbert, Frank' },
};

// Everything on screen that shows an author has to be asked again when the order changes.
function refreshNames(queryClient) {
  for (const key of ['preferences', 'works', 'work', 'favorites', 'search']) {
    queryClient.invalidateQueries({ queryKey: [key] });
  }
}

/** { choice, library, effective }: the account's own choice ('' for none), the library's default, what applies. */
export function usePreferences(enabled = true) {
  return useQuery({
    queryKey: ['preferences'],
    queryFn: async () => (await api.get('/auth/preferences')).data,
    enabled,
    staleTime: 60_000,
  });
}

/** An account chooses its own order; '' goes back to the library's default. */
export function useSetNameOrder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (nameOrder) => (await api.put('/auth/preferences', { nameOrder })).data,
    onSuccess: () => refreshNames(queryClient),
  });
}

/** The owner sets the library's default. */
export function useSetLibraryNameOrder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (nameOrder) => (await api.put('/admin/name-order', { nameOrder })).data,
    onSuccess: () => refreshNames(queryClient),
  });
}
