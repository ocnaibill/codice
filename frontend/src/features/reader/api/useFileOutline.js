import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * The table of contents of a file as the text index has it, with where each entry opens and the one the person's saved place is in
 * (DEC-150). Empty for a file with no text index or no outline; the page then says nothing of it.
 */
export function useFileOutline(fileId, { enabled = true } = {}) {
  return useQuery({
    queryKey: ['file-outline', fileId],
    queryFn: async () => (await api.get(`/progress/files/${fileId}/outline`)).data,
    enabled: !!fileId && enabled,
    staleTime: 0,
  });
}
