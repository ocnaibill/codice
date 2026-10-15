import { useQuery, keepPreviousData } from '@tanstack/react-query';
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
