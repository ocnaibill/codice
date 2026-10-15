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

/** What the staff decided about who is who in this work or series (DEC-181). */
export function useWikiIdentity(projectId, enabled = true) {
  return useQuery({
    queryKey: ['wiki-identity', projectId],
    queryFn: async () => (await api.get(`/wiki/${projectId}/identity`)).data,
    enabled: !!projectId && enabled,
  });
}

/** Joins two entities, keeps them apart, or takes a mention away from one. The answer is the slug the page goes on to show. */
export function useDecideIdentity(projectId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (decision) => (await api.post(`/wiki/${projectId}/identity`, decision)).data,
    onSuccess: () => {
      for (const key of ['wiki', 'wiki-entity', 'wiki-graph', 'wiki-review', 'wiki-identity']) queryClient.invalidateQueries({ queryKey: [key] });
    },
  });
}

/** Takes a decision back for the next generations (what was done in the wiki that exists stays until it is generated again). */
export function useForgetIdentity(projectId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (ruleId) => (await api.delete(`/wiki/${projectId}/identity/${ruleId}`)).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['wiki-identity'] }),
  });
}

const MANUAL_KEYS = ['wiki', 'wiki-entity', 'wiki-graph', 'wiki-review', 'wiki-manual'];
const refreshManual = (queryClient) => {
  for (const key of MANUAL_KEYS) queryClient.invalidateQueries({ queryKey: [key] });
};

/** What the staff wrote by hand about an entity (DEC-186): its page, its moments, its ties and its edges. */
export function useWikiManual(projectId, slug, enabled = true) {
  return useQuery({
    queryKey: ['wiki-manual', projectId, slug],
    queryFn: async () => (await api.get(`/wiki/${projectId}/manual`, { params: { entity: slug } })).data,
    enabled: !!projectId && !!slug && enabled,
  });
}

/** Writes items by hand, all or nothing; `"@0"` in `entity` or `other` is the entity the item 0 of the same request creates. */
export function useCreateManual(projectId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (items) => (await api.post(`/wiki/${projectId}/manual`, { items })).data,
    onSuccess: () => refreshManual(queryClient),
  });
}

export function useUpdateManual(projectId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, ...item }) => (await api.put(`/wiki/${projectId}/manual/${id}`, item)).data, // the id is in the address
    onSuccess: () => refreshManual(queryClient),
  });
}

export function useDeleteManual(projectId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id) => (await api.delete(`/wiki/${projectId}/manual/${id}`)).data,
    onSuccess: () => refreshManual(queryClient),
  });
}
