import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const ACTIVE = ['pending', 'running'];
export const isActive = (job) => !!job && ACTIVE.includes(job.state);
/** Two seconds while the search is waiting or running, and never once it is over. */
export const pollInterval = (job) => (isActive(job) ? 2000 : false);

/** How the last search of the providers for a work went (owner and admin): waiting, running, done with what it found, or failed.
 *  It asks again every two seconds while the search is waiting or running. */
export function useMetadataRefreshStatus(workId, { enabled = true } = {}) {
  return useQuery({
    queryKey: ['metadata-refresh', workId],
    queryFn: async () => (await api.get(`/admin/works/${workId}/metadata-refresh`)).data.job,
    enabled: enabled && !!workId,
    refetchInterval: (query) => pollInterval(query.state.data),
  });
}

/** Asks for the providers to be searched again for the work. What they answer comes back as suggestions (DEC-143). */
export function useRefreshMetadata(workId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/admin/works/${workId}/metadata-refresh`)).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['metadata-refresh', workId] }),
  });
}
