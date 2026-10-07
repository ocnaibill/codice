import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';
import { serverMessage } from '../../../lib/serverMessage';

/** The collections the person sees, by name (#184). With `retired`, the retired ones, which only owner and admin are given. */
export function useCollections({ page = 1, limit = 24, retired = false } = {}) {
  return useQuery({
    queryKey: ['collections', { page, limit, retired }],
    queryFn: async () => {
      const params = { page, limit };
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

export function useCreateCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: async (name) => (await api.post('/collections', { name })).data, // { id, name }
    onSuccess: refresh,
  });
}

export function useRenameCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: async ({ id, name }) => (await api.patch(`/collections/${id}`, { name })).data,
    onSuccess: refresh,
  });
}

/** The work joins the collection (at the end, or at `position`); one that was in another official collection leaves it. */
export function useAddToCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ id, workId, position }) =>
      api.put(`/collections/${id}/works/${workId}`, position == null ? {} : { position }),
    onSuccess: refresh,
  });
}

export function useRemoveFromCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ id, workId }) => api.delete(`/collections/${id}/works/${workId}`),
    onSuccess: refresh,
  });
}

/** The works get the numbers 1, 2, 3… in the order of `workIds`, which must be all the works of the collection. */
export function useOrderCollection() {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ id, workIds }) => api.put(`/collections/${id}/order`, { workIds }),
    onSuccess: refresh,
  });
}

export function useRetireCollection() {
  const refresh = useRefreshing();
  return useMutation({ mutationFn: (id) => api.delete(`/collections/${id}`), onSuccess: refresh });
}

export function useRestoreCollection() {
  const refresh = useRefreshing();
  return useMutation({ mutationFn: (id) => api.post(`/collections/${id}/restore`), onSuccess: refresh });
}
