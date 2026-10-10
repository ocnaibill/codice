import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * The person's own highlights and notes on the works of a collection, each with the chapter its place is in when the file has an outline
 * (DEC-163). Asked only when the page says there are some. The key starts with 'notes', so that saving or deleting one in the reader
 * refreshes this too.
 */
export const useCollectionNotes = (collectionId, enabled = true) =>
  useQuery({
    queryKey: ['notes', 'collection', collectionId, 'highlights'],
    queryFn: async () => (await api.get('/notes', { params: { collectionId, limit: 200, chapters: true } })).data,
    enabled: !!collectionId && enabled,
    staleTime: 0,
  });
