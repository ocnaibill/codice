import { useEffect, useState } from 'react';
import { serverMessage } from '../../../lib/serverMessage';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/** The works whose title or author match what was typed, for choosing where to put a work (#37). */
export function useWorkSearch(term, { limit = 8 } = {}) {
  const [debounced, setDebounced] = useState(term);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(term), 250);
    return () => clearTimeout(timer);
  }, [term]);
  const search = debounced.trim();
  return useQuery({
    queryKey: ['works', { versionsSearch: search, limit }],
    queryFn: async () => (await api.get('/works', { params: { search, limit } })).data,
    enabled: search.length >= 2,
  });
}

/** Puts every edition of one work under another (owner and admin). The work left empty is retired. */
export function useJoinWork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ workId, into }) => (await api.post(`/admin/works/${workId}/join`, { into })).data,
    onSuccess: () => refreshLibrary(queryClient),
  });
}

/** Takes an edition out of its work: back to the one it came from, or to a new one. */
export function useSplitEdition() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ editionId }) => (await api.post(`/admin/editions/${editionId}/split`)).data,
    onSuccess: () => refreshLibrary(queryClient),
  });
}

/** Records that two works are not the same book, so they are never proposed to be joined. */
export function useNotTheSame() {
  return useMutation({
    mutationFn: ({ workId, otherId }) => api.post(`/admin/works/${workId}/not-same-as`, { workId: otherId }),
  });
}

/** What the server said, for showing; it answers with plain text. */
export function reasonOf(error, fallback) {
  return serverMessage(error, fallback);
}
