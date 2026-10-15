import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * The query of a cursor the reader chose (DEC-179): a point beyond or before their own (book and chapter) and whether the side books
 * count. With none, the server uses the reader's own place, from their reading progress.
 */
export function cursorParams(cursor) {
  const params = {};
  if (cursor?.manual) {
    params.book = cursor.book;
    params.chapter = cursor.chapter;
  }
  if (cursor?.side) params.side = 1;
  return params;
}

const keyOf = (cursor) => [cursor?.manual ? `${cursor.book}:${cursor.chapter}` : 'own', cursor?.side ? 'side' : 'main'];

/** The wiki of a work (RF-051): its books for the cursor control, the cursor and the index up to it; `{ wiki: null }` when it has none. */
export function useWiki(workId, cursor = null, { enabled = true } = {}) {
  return useQuery({
    queryKey: ['wiki', workId, ...keyOf(cursor)],
    queryFn: async () => (await api.get(`/works/${workId}/wiki`, { params: cursorParams(cursor) })).data,
    enabled: !!workId && enabled,
    placeholderData: keepPreviousData,
  });
}

/** The page of one entity up to the cursor. An entity the reader has not met yet is a 404. */
export function useWikiEntity(projectId, slug, cursor = null) {
  return useQuery({
    queryKey: ['wiki-entity', projectId, slug, ...keyOf(cursor)],
    queryFn: async () => (await api.get(`/wiki/${projectId}/entities/${encodeURIComponent(slug)}`, { params: cursorParams(cursor) })).data,
    enabled: !!projectId && !!slug,
    retry: (count, error) => error?.response?.status !== 404 && count < 2,
  });
}

/** The graph of the wiki up to the cursor: the nodes that appeared and the edges between them. */
export function useWikiGraph(projectId, cursor = null, { enabled = true } = {}) {
  return useQuery({
    queryKey: ['wiki-graph', projectId, ...keyOf(cursor)],
    queryFn: async () => (await api.get(`/wiki/${projectId}/graph`, { params: cursorParams(cursor) })).data,
    enabled: !!projectId && enabled,
    placeholderData: keepPreviousData,
  });
}

/** What the last update of the wiki changed, for the staff (DEC-180). Not in the summary: a later book's names are spoilers. */
export function useWikiReview(projectId, enabled = true) {
  return useQuery({
    queryKey: ['wiki-review', projectId],
    queryFn: async () => (await api.get(`/wiki/${projectId}/review`)).data,
    enabled: !!projectId && enabled,
  });
}

const refreshWiki = (queryClient) => {
  for (const key of ['wiki', 'wiki-entity', 'wiki-graph', 'wiki-review']) queryClient.invalidateQueries({ queryKey: [key] });
};

/** The review of an update: `approve` closes it, `undo` hides what one entity gained ({ slug }). */
export function useWikiReviewActions(projectId) {
  const queryClient = useQueryClient();
  const approve = useMutation({
    mutationFn: async () => (await api.post(`/wiki/${projectId}/review/approve`)).data,
    onSuccess: () => refreshWiki(queryClient),
  });
  const undo = useMutation({
    mutationFn: async (slug) => (await api.post(`/wiki/${projectId}/review/undo`, { slug })).data,
    onSuccess: () => refreshWiki(queryClient),
  });
  return { approve, undo };
}

/** Marks a sentence as wrong (no reader gets it) or shows it again ({ id, hidden }). */
export function useHideSentence(projectId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, hidden }) => (await api.put(`/wiki/${projectId}/sentences/${id}/hidden`, { hidden })).data,
    onSuccess: () => refreshWiki(queryClient),
  });
}
