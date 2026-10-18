import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * The two places of a conflict (the web's and the one a device sent that was kept apart, DEC-191), each said in words: how far, when, who,
 * the chapter and the passage. Asked only while the person is choosing.
 */
export function usePlaceComparison(fileId, enabled = true) {
  return useQuery({
    queryKey: ['place-comparison', fileId],
    queryFn: async () => (await api.get(`/progress/files/${fileId}/compare`)).data,
    enabled: !!fileId && enabled,
    staleTime: 0,
    gcTime: 0,
  });
}
