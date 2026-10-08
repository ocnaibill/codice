import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';
import { serverMessage } from '../../../lib/serverMessage';

/** Where the management of a kind of collection lives: the official ones are the staff's, the lists are each person's (#207). */
const base = (kind) => (kind === 'personal' ? '/my/collections' : '/collections');

/**
 * The collections the person sees, by name (#184): the official ones, or with kind "personal" their own lists (#207). With
 * `retired`, the retired ones: owner and admin are given the official ones, and anybody the lists they put away.
 */
export function useCollections({ page = 1, limit = 24, retired = false, kind = 'official' } = {}) {
  return useQuery({
    queryKey: ['collections', { page, limit, retired, kind }],
    queryFn: async () => {
      const params = { page, limit };
      if (kind === 'personal') params.kind = 'personal';
      if (retired) params.retired = 'true';
      return (await api.get('/collections', { params })).data; // { data, total, page, limit, totalPages }
    },
  });
}

/** One collection and its works, in order. */
export function useCollection(id) {
  return useQuery({
    queryKey: ['collection', id],
    queryFn: async () => (await api.get(`/collections/${id}`)).data, // { collection, works }
    enabled: !!id,
  });
}

/** What a refusal of the management says: the server puts it in `error` when it also says which collection it means. */
export function collectionReason(error, fallback) {
  const said = error?.response?.data;
  if (said && typeof said === 'object' && typeof said.error === 'string') return said.error;
  return serverMessage(error, fallback);
}

// A change of a collection is a change of the series of works, so what shows works and collections is stale together
// (refreshLibrary knows both).
function useRefreshing() {
  const queryClient = useQueryClient();
  return () => refreshLibrary(queryClient);
}

// Every change takes the `kind` of the collection, since the lists of a person and the official ones are managed in other places.

export function useCreateCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: async ({ name, kind }) => (await api.post(base(kind), { name })).data, // { id, name }
    onSuccess: refresh,
  });
}

export function useRenameCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: async ({ id, name, kind }) => (await api.patch(`${base(kind)}/${id}`, { name })).data,
    onSuccess: refresh,
  });
}

/** The work joins the collection (at the end, or at `position`); an official one that was in another collection leaves it. */
export function useAddToCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ id, workId, position, kind }) =>
      api.put(`${base(kind)}/${id}/works/${workId}`, position == null ? {} : { position }),
    onSuccess: refresh,
  });
}

/** `work` is the place as the page of the collection gives it: an official collection names the work, a list names the place,
 *  since the work of a place may be gone. */
export function useRemoveFromCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ id, work, kind }) =>
      api.delete(kind === 'personal' ? `/my/collections/${id}/entries/${work.entryId}` : `/collections/${id}/works/${work.id}`),
    onSuccess: refresh,
  });
}

/** The works (the places of a list) get the numbers 1, 2, 3… in the order of `items`, which must be all of them. */
export function useOrderCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ id, items, kind }) =>
      api.put(`${base(kind)}/${id}/order`, kind === 'personal' ? { entryIds: items.map((i) => i.entryId) } : { workIds: items.map((i) => i.id) }),
    onSuccess: refresh,
  });
}

export function useRetireCollection() {
  const refresh = useRefreshing();
  return useMutation({ mutationFn: ({ id, kind }) => api.delete(`${base(kind)}/${id}`), onSuccess: refresh });
}

export function useRestoreCollection() {
  const refresh = useRefreshing();
  return useMutation({ mutationFn: ({ id, kind }) => api.post(`${base(kind)}/${id}/restore`), onSuccess: refresh });
}
