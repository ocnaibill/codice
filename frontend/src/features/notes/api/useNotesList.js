import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

export const PAGE_SIZE = 20;

/** The filters of the list as the API takes them: only what was set. */
export function filterParams({ q, kind, tag, workId } = {}) {
  const params = new URLSearchParams();
  if (q?.trim()) params.set('q', q.trim());
  if (kind) params.set('kind', kind);
  if (tag) params.set('tag', tag);
  if (workId) params.set('workId', String(workId));
  return params;
}

/** A page of the person's own notes, newest first, narrowed by the filters. The key starts with 'notes', so
 *  saving, editing or deleting a note anywhere refreshes it. */
export const useNotesList = (filters, page) =>
  useQuery({
    queryKey: ['notes', 'list', { ...filters, page }],
    queryFn: async () => {
      const params = filterParams(filters);
      params.set('limit', String(PAGE_SIZE));
      params.set('offset', String((page - 1) * PAGE_SIZE));
      return (await api.get(`/notes?${params}`)).data;
    },
    placeholderData: keepPreviousData, // the page stays while the next one loads
    staleTime: 0,
  });
