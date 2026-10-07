import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { deviceClass } from '../../reader/deviceClass';

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

/** How the person wants to be called (#179): '' goes back to the user name. The answer is also "asked": the first-sign-in question is not repeated. */
export function useSetDisplayName() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (displayName) => (await api.put('/auth/preferences', { displayName })).data,
    onSuccess: () => {
      for (const key of ['me', 'preferences']) queryClient.invalidateQueries({ queryKey: [key] });
    },
  });
}

/** The most characters a name to be called by may have (the server holds the same limit). */
export const MAX_DISPLAY_NAME = 60;

/**
 * Whether how the text looks is kept the same on every device (#180): on, it takes this device's choice to the others; off, each
 * kind of device keeps what it has and goes its own way.
 */
export function useSetReadingShared() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (shared) =>
      (await api.put('/auth/preferences', { reader: shared ? { shared: true, from: deviceClass() } : { shared: false } })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['preferences'] }),
  });
}
