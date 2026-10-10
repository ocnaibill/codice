import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * A few passages of a file in a row, from its text index (DEC-153): the window that holds the person's place, or the one that starts at
 * `from`. Empty for a file with no text index; the page then says nothing of it.
 */
export function useFilePreview(fileId, { from, enabled = true } = {}) {
  return useQuery({
    queryKey: ['file-preview', fileId, from ?? 'place'],
    queryFn: async () => (await api.get(`/progress/files/${fileId}/preview`, { params: from == null ? {} : { from } })).data,
    enabled: !!fileId && enabled,
    staleTime: 0,
    // the window on screen stays while the next arrives: the text does not flash
    placeholderData: keepPreviousData,
  });
}
