import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { filterParams } from './useNotesList';

/** What the screen of notes offers to narrow the list by, with the count of each: the notes of each kind and the tags in
 *  use. It follows the filters of the list (each facet ignores its own choice, so it shows what the others would give).
 *  The key starts with 'notes', so saving, editing or deleting a note refreshes it. */
export const useNotesFacets = (filters) =>
  useQuery({
    queryKey: ['notes', 'facets', filters],
    queryFn: async () => (await api.get(`/notes/facets?${filterParams(filters)}`)).data,
    placeholderData: keepPreviousData, // the panel stays while the counts are read again
    staleTime: 0,
  });
