import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** Turns an error from the API into a sentence a person can act on. */
export function describeError(error) {
  const res = error?.response;
  if (!res) return 'Não foi possível falar com o servidor.';
  if (typeof res.data === 'string' && res.data.trim()) return res.data.trim();
  if (res.status === 403) return 'Você não tem permissão para isso.';
  return 'Algo deu errado.';
}

const list = (key, url, params) =>
  function useAdminList(options = {}) {
    return useQuery({
      queryKey: ['admin', key, params?.(options) ?? null],
      queryFn: async () => (await api.get(url, { params: params?.(options) })).data,
      staleTime: 0,
    });
  };

export const useJobs = list('jobs', '/admin/jobs', ({ state } = {}) => (state ? { state } : undefined));
export const useRoots = list('roots', '/admin/storage/roots');
export const useCleanups = list('cleanups', '/admin/storage/cleanups');
export const useOrphans = list('orphans', '/admin/storage/orphans');
export const useTrash = list('trash', '/admin/trash');
export const useDuplicates = list('duplicates', '/admin/duplicates');
export const useOcr = list('ocr', '/admin/ocr');

/** A POST/PUT/DELETE that refreshes the admin lists when it succeeds. */
function useAdminAction(run) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin'] }),
  });
}

export const useRerunJob = () => useAdminAction((id) => api.post(`/admin/jobs/${id}/rerun`));
export const useCancelJob = () => useAdminAction((id) => api.post(`/admin/jobs/${id}/cancel`));

export const useAddRoot = () => useAdminAction((path) => api.post('/admin/storage/roots', { path }));
export const useRemoveRoot = () => useAdminAction((id) => api.delete(`/admin/storage/roots/${id}`));
export const useScanRoot = () => useAdminAction((rootId) => api.post('/admin/library/scan', { rootId }));
export const useRetryCleanups = () => useAdminAction(() => api.post('/admin/storage/cleanups/retry'));
export const useTrashOrphans = () =>
  useAdminAction((paths) => api.post('/admin/storage/orphans/trash', { paths }));

export const useReorganizePreview = () =>
  useMutation({ mutationFn: async () => (await api.get('/admin/storage/reorganize')).data });
export const useReorganize = () =>
  useAdminAction(async (hash) => (await api.post('/admin/storage/reorganize', { hash })).data);

/** Bulk import of a server-side folder. `removeOriginals` is always chosen by the person. */
export const useBulkImport = () =>
  useAdminAction(async ({ directory, removeOriginals }) =>
    (await api.post('/works/bulk-import', { directory, removeOriginals }, { timeout: 0 })).data);

export const useRestoreTrash = () => useAdminAction((id) => api.post(`/admin/trash/${id}/restore`));
export const useDeleteTrash = () => useAdminAction((id) => api.delete(`/admin/trash/${id}`, { params: { confirm: true } }));
export const useEmptyTrash = () => useAdminAction(() => api.post('/admin/trash/empty', { confirm: true }));
export const useSetTrashPolicy = () => useAdminAction((policy) => api.put('/admin/trash/policy', policy));
export const usePreviewTrashPolicy = () =>
  useMutation({ mutationFn: async (days) => (await api.get('/admin/trash/policy/preview', { params: { days } })).data });
export const useApplyTrashPolicy = () =>
  useAdminAction((days) => api.post('/admin/trash/policy/apply', { days, confirm: true }));

export const useScanDuplicates = () => useAdminAction(() => api.post('/admin/duplicates/scan'));
export const useDismissDuplicate = () => useAdminAction((id) => api.post(`/admin/duplicates/${id}/dismiss`));
export const useLinkDuplicate = () =>
  useAdminAction(({ id, keep }) => api.post(`/admin/duplicates/${id}/link`, { keep, confirm: true }));
