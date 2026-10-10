import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * The person's own highlights and notes in one work, each with the chapter its place is in when the file has an outline (DEC-151). The key
 * starts with 'notes', so that saving or deleting one in the reader refreshes this too.
 */
export const useWorkHighlights = (workId) =>
  useQuery({
    queryKey: ['notes', 'work', workId, 'highlights'],
    queryFn: async () => (await api.get('/notes', { params: { workId, limit: 200, chapters: true } })).data,
    enabled: !!workId,
    staleTime: 0,
  });
