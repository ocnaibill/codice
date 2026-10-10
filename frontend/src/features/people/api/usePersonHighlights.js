import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * The person's own highlights and notes on the works of another person (an author, a translator), each with the chapter its place is in when
 * the file has an outline (DEC-158). The key starts with 'notes', so that saving or deleting one in the reader refreshes this too.
 */
export const usePersonHighlights = (personId) =>
  useQuery({
    queryKey: ['notes', 'person', personId, 'highlights'],
    queryFn: async () => (await api.get('/notes', { params: { personId, limit: 200, chapters: true } })).data,
    enabled: !!personId,
    staleTime: 0,
  });
