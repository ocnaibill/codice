import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/** What the providers suggested for a work and nobody has decided yet (owner and admin). */
export function useCandidates(workId, { enabled = true } = {}) {
  return useQuery({
    queryKey: ['candidates', workId],
    queryFn: async () => (await api.get(`/works/${workId}/candidates`)).data.data,
    enabled: enabled && !!workId,
  });
}

/** Accepts or rejects one suggestion. Accepting changes the work, may join the author to a person who may be
 *  the same as another, so everything derived from the catalogue is asked for again. */
export function useDecideCandidate(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, verb }) => (await api.post(`/works/${workId}/candidates/${id}/${verb}`)).data,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['candidates', workId] });
      // The admin lists: the queue of suggestions, and the people who may be the same (an accepted author).
      queryClient.invalidateQueries({ queryKey: ['admin'] });
      refreshLibrary(queryClient);
    },
  });
}
