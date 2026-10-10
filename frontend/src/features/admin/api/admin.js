import { keepPreviousData, useInfiniteQuery, useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { messageOf } from '../../../lib/serverMessage';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';
import { POLL_MS } from '../systemLimits';

/** Turns an error from the API into a sentence a person can act on. */
export function describeError(error) {
  const res = error?.response;
  if (!res) return 'Não foi possível falar com o servidor.';
  const said = res.status < 500 ? messageOf(res.data) : ''; // what the server said that a person can use, in Portuguese
  if (said) return said;
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
/** What the "Sistema" tab shows: the last backup, the queue, the space and the owner's two backup buttons. While a job of
 *  those buttons is live it asks again every few seconds, so the screen follows it to the end. */
export function useBackup() {
  return useQuery({
    queryKey: ['admin', 'backup', null],
    queryFn: async () => (await api.get('/admin/backup')).data,
    staleTime: 0,
    refetchInterval: (query) => {
      const state = query.state.data?.panel?.job?.state;
      return state === 'pending' || state === 'running' ? POLL_MS : false;
    },
  });
}

// The owner makes a package, or checks one of the folder, from the panel (DEC-123). The password is asked again each time.
function usePanelAction(path) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body) => (await api.post(path, body)).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'backup'] }),
  });
}
export const useRunBackup = () => usePanelAction('/admin/backup/run');
export const useVerifyBackup = () => usePanelAction('/admin/backup/verify');
/** The public health of the API by component (RF-021). The answer is the same JSON when something is down (HTTP 503), so a
 *  down database is shown, not treated as a failure to load. It asks again every half minute while the tab is open. */
export function useHealth() {
  return useQuery({
    queryKey: ['admin', 'health'],
    queryFn: async () => (await api.get('/healthz', { validateStatus: () => true })).data,
    staleTime: 0,
    refetchInterval: 30000,
  });
}
export const useOrphans = list('orphans', '/admin/storage/orphans');
export const useTrash = list('trash', '/admin/trash');
/** The works that were retired from the catalog, the latest first, a page at a time: they wait to be restored or to have their files sent to the trash. */
export function useRetiredWorks(page = 1) {
  return useQuery({
    queryKey: ['admin', 'retired-works', page],
    queryFn: async () => (await api.get('/admin/retired-works', { params: { page, limit: 20 } })).data, // { data, total, page, limit, totalPages }
    staleTime: 0,
    placeholderData: keepPreviousData,
  });
}
/** The pairs waiting for a decision, a page at a time (an instance with thousands of works has over a thousand of them). Each
 *  page carries the pairs, how many wait in all (`total`) and whether there are more (`more`). */
export function useDuplicates() {
  return useInfiniteQuery({
    queryKey: ['admin', 'duplicates', null],
    queryFn: async ({ pageParam }) => (await api.get('/admin/duplicates', { params: pageParam ? { after: pageParam } : undefined })).data,
    initialPageParam: 0,
    getNextPageParam: (last) => (last.more && last.data.length ? last.data[last.data.length - 1].id : undefined),
    staleTime: 0,
  });
}
export const useSuggestionQueue = list('suggestion-queue', '/admin/suggestions');
/** Asks again every few seconds while a test of a provider is waiting or running, and never otherwise. */
export const pollWhileTesting = (query) => (query.state.data?.data?.some((provider) => provider.testing) ? POLL_MS : false);

/** The external metadata providers, with how each answered and how its last test came out. While a test is waiting or running it asks again
 *  every few seconds, so the screen follows it to the end (DEC-145). */
export function useMetadataProviders() {
  return useQuery({
    queryKey: ['admin', 'metadata-providers', null],
    queryFn: async () => (await api.get('/admin/metadata-providers')).data,
    staleTime: 0,
    refetchInterval: pollWhileTesting,
  });
}
export const usePeopleMerges = list('people-merges', '/admin/people/merges');
/** Whether OCR is on, which engine there is and what it is doing. While it is on, it asks again every few seconds: the
 *  service takes a moment to notice that it was turned on, and then to say it is working. */
export function useOcrSettings() {
  return useQuery({
    queryKey: ['admin', 'ocr-settings', null],
    queryFn: async () => (await api.get('/admin/ocr/settings')).data,
    staleTime: 0,
    refetchInterval: (query) => (query.state.data?.enabled ? 3000 : false),
  });
}

/** The PDFs with pages that have no text, and how far reading them by OCR has got. It asks again every few seconds while
 *  OCR is on (`live`: the work is queued a moment after it is turned on, so nothing says "waiting" yet) and while a PDF
 *  is waiting or being read, so the numbers move on their own. */
export function useOcr({ live = false } = {}) {
  return useQuery({
    queryKey: ['admin', 'ocr', null],
    queryFn: async () => (await api.get('/admin/ocr')).data,
    staleTime: 0,
    refetchInterval: (query) => (live || query.state.data?.data?.some((item) => item.state === 'queued' || item.state === 'reading') ? 3000 : false),
  });
}
/** The dictionaries the owner may install and where each is. While one is being installed it asks again every two seconds, so
 *  the download and the import move on their own. */
export function useDictionaries() {
  return useQuery({
    queryKey: ['admin', 'dictionaries', null],
    queryFn: async () => (await api.get('/admin/dictionaries')).data,
    staleTime: 0,
    refetchInterval: (query) => (query.state.data?.packages?.some((p) => p.state === 'installing') ? 2000 : false),
  });
}
export const useAccounts = list('accounts', '/users');
export const useLdap = list('ldap', '/admin/ldap');
export const useEmbeddings = list('embeddings', '/admin/embeddings');
export const useInvitations = list('invitations', '/invitations');
export const usePasswordResets = list('password-resets', '/password-resets');

export const REFERENCED_PAGE = 50;

/** A page of the referenced files, narrowed by directory, state and text (#15). While a transfer of one of them is
 *  waiting or running, it asks again, so the state moves on its own. */
export function useReferenced({ rootId, state, q, page }, { live = false } = {}) {
  return useQuery({
    queryKey: ['admin', 'referenced', { rootId, state, q, page }],
    queryFn: async () => {
      const params = { limit: REFERENCED_PAGE, offset: (page - 1) * REFERENCED_PAGE };
      if (rootId) params.rootId = rootId;
      if (state) params.state = state;
      if (q?.trim()) params.q = q.trim();
      return (await api.get('/admin/storage/referenced', { params })).data;
    },
    placeholderData: keepPreviousData,
    staleTime: 0,
    // Asks again while something that changes it is going on: a transfer of one of its files, or a scan (`live`).
    refetchInterval: (query) =>
      live || query.state.data?.data?.some((f) => f.transfer && (f.transfer.state === 'pending' || f.transfer.state === 'running')) ? 3000 : false,
  });
}

/** Whether a scan of a folder is waiting or running. A scan ends after it was asked for, so the screens that show
 *  what it catalogues ask again, every couple of seconds, while this is true. Asks only for scans, so a flood of other
 *  jobs (the ingestion of the books it finds) cannot push one out of the page. */
export function useScanActivity() {
  return useQuery({
    queryKey: ['admin', 'scan-activity'],
    queryFn: async () => ((await api.get('/admin/jobs', { params: { type: 'scan', limit: 10 } })).data.data || []).some((j) => j.state === 'pending' || j.state === 'running'),
    staleTime: 0,
    refetchInterval: (query) => (query.state.data ? 2500 : false),
  });
}

/** Asks for the transfer of referenced files into the managed storage: one job per file, or the reason it was not queued. */
export const useMoveToManaged = () =>
  useAdminAction(async (fileIds) => (await api.post('/admin/library/move-to-managed', { fileIds })).data);

/** How the transfers asked for are going, per job. It asks again every two seconds until none is waiting or running. */
export function useTransfers(jobIds) {
  const ids = [...jobIds].sort((a, b) => a - b);
  return useQuery({
    queryKey: ['admin', 'transfers', ids],
    queryFn: async () => (await api.get('/admin/storage/transfers', { params: { ids: ids.join(',') } })).data,
    enabled: ids.length > 0,
    staleTime: 0,
    refetchInterval: (query) => {
      const rows = query.state.data?.data;
      return !rows || rows.some((t) => t.outcome === 'queued' || t.outcome === 'running') ? 2000 : false;
    },
  });
}

/** A POST/PUT/DELETE that refreshes the admin lists when it succeeds. */
function useAdminAction(run) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin'] }),
  });
}

/** Asks for one provider to be tested: a fixed, public question, whether it is on or not (owner, DEC-145). */
export const useTestMetadataProvider = () => useAdminAction((id) => api.post(`/admin/metadata-providers/${id}/test`));

/** Turns an external metadata provider on or off (owner). */
export const useSetMetadataProvider = () =>
  useAdminAction(({ id, enabled }) => api.put(`/admin/metadata-providers/${id}`, { enabled }));

export const useRerunJob = () => useAdminAction((id) => api.post(`/admin/jobs/${id}/rerun`));
/** Puts every failed job of a work back in the queue; answers with how many went and how many failed jobs are left. */
export const useRerunFailedJobs = () => useAdminAction(async () => (await api.post('/admin/jobs/rerun-failed')).data);
export const useCancelJob = () => useAdminAction((id) => api.post(`/admin/jobs/${id}/cancel`));

export const useAddRoot = () => useAdminAction((path) => api.post('/admin/storage/roots', { path }));
export const useRemoveRoot = () => useAdminAction((id) => api.delete(`/admin/storage/roots/${id}`));
/** Deletes for good the retired works of a folder that keep no bytes on the server (only their records go). The server does a few
 *  hundred a call, so this asks again until none waits and reports each step to `onProgress(purged so far)`; it answers with how many
 *  it deleted. What was deleted before an error stays deleted, so the lists are asked again either way. */
export function usePurgeRetiredInRoot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, onProgress }) => {
      let purged = 0;
      for (;;) {
        const { purged: step, remaining } = (await api.post(`/admin/storage/roots/${id}/purge-retired`)).data;
        purged += step;
        onProgress?.(purged);
        // A call that deleted nothing is the end, whatever it says waits: asking again would not change that.
        if (remaining === 0 || step === 0) return purged;
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['admin'] }),
  });
}
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

/** A retired work comes back to the catalog (staff). What shows works and counts is refreshed with the lists of the administration. */
export function useRestoreWork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id) => api.post(`/works/${id}/restore`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin'] });
      refreshLibrary(queryClient);
    },
  });
}
/** "Delete for good" of a retired work: its files go to the trash, where they can still be recovered (staff). Answers with how many files went and whether the record is gone. */
export const usePurgeWork = () => useAdminAction(async (id) => (await api.delete(`/works/${id}`, { params: { purge: true } })).data);
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

export const useMergePeople = () =>
  useAdminAction(({ id, keep }) => api.post(`/admin/people/merges/${id}/merge`, { keep, confirm: true }));
export const useDismissPeopleMerge = () => useAdminAction((id) => api.post(`/admin/people/merges/${id}/dismiss`));

/** The people whose surname is still to be told apart (`state` unset), or the ones already dealt with (`state: 'done'`), a page at a time,
 *  the ones with most works first. Each carries the division proposed (or, dealt with, the one it has). */
export function useNames({ state, q = '', page = 1 }) {
  return useQuery({
    queryKey: ['admin', 'names', state ?? 'pending', q, page],
    queryFn: async () => (await api.get('/admin/people/names', { params: { ...(state ? { state } : {}), ...(q ? { q } : {}), page } })).data, // { data, total, page, limit, totalPages }
    staleTime: 0,
    placeholderData: keepPreviousData,
  });
}
/** Tells apart the surname of some people, one after the other, each as `{ id, family, given }` (`{ id, undivided: true }` says that there is
 *  none, `{ id, family: '', given: '' }` takes the division back). It reports each one to `onProgress(how many so far)`, and stops at the first
 *  the server refuses; what was saved stays saved, so the lists are asked again either way, the ones of the library included (the name is
 *  shown in the order a person prefers once the surname is known). */
export function useDivideNames() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ items, onProgress }) => {
      let saved = 0;
      for (const { id, family = '', given = '', undivided = false } of items) {
        await api.put(`/admin/people/${id}/name`, undivided ? { undivided: true } : { family, given });
        saved += 1;
        onProgress?.(saved);
      }
      return saved;
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['admin'] });
      refreshLibrary(queryClient);
    },
  });
}

export const useBlockAccount = () => useAdminAction((id) => api.post(`/users/${id}/block`));
export const useUnblockAccount = () => useAdminAction((id) => api.post(`/users/${id}/unblock`));

export const useCreateInvitation = () =>
  useAdminAction(async ({ role, email }) => (await api.post('/invitations', { role, email })).data);
export const useRevokeInvitation = () => useAdminAction((id) => api.delete(`/invitations/${id}`));
export const useDeleteAccount = () =>
  useAdminAction(({ id, username }) => api.delete(`/users/${id}`, { data: { confirmUsername: username } }));
export const useApproveReset = () => useAdminAction(async (id) => (await api.post(`/password-resets/${id}/approve`)).data);
export const useRejectReset = () => useAdminAction((id) => api.post(`/password-resets/${id}/reject`));
export const useSetLdapPolicy = () => useAdminAction(async (policy) => (await api.put('/admin/ldap/policy', policy)).data);
export const useSetOcr = () => useAdminAction(async (settings) => (await api.put('/admin/ocr/settings', settings)).data);
/** Asks for the pages of a work that failed to be read to be tried again. */
export const useRetryOcr = () => useAdminAction(async (workId) => (await api.post(`/admin/works/${workId}/ocr/retry`)).data);
/** Says the language a scanned file is read in was wrong: its pages are read again, from the first, in the one given. */
export const useSetOcrLanguage = () => useAdminAction(async ({ fileId, language }) => (await api.post(`/admin/files/${fileId}/ocr/language`, { language })).data);
/** Installs a dictionary (the server downloads it and imports it), cancels the installation, or removes it (owner). */
export const useInstallDictionary = () => useAdminAction(async (id) => (await api.post(`/admin/dictionaries/${id}/install`)).data);
export const useCancelDictionary = () => useAdminAction(async (id) => (await api.post(`/admin/dictionaries/${id}/cancel`)).data);
export const useRemoveDictionary = () => useAdminAction((id) => api.delete(`/admin/dictionaries/${id}`));
export const useSetEmbeddings = () => useAdminAction(async (settings) => (await api.put('/admin/embeddings', settings)).data);
export const useCheckLdap = () => useMutation({ mutationFn: async () => (await api.post('/admin/ldap/check')).data });

/** The record of sign-ins (DEC-121), a page at a time, newest first. `filters` is { result, username, from }; each page
 *  carries the entries, whether there is more, how long the record is kept and whether every entry comes from one
 *  address of the private network (a proxy hiding the clients). */
export function useLoginEvents(filters = {}) {
  return useInfiniteQuery({
    queryKey: ['admin', 'logins', filters.result ?? '', filters.username ?? '', filters.from ?? ''],
    queryFn: async ({ pageParam }) => {
      const params = { limit: 30 };
      if (filters.result) params.result = filters.result;
      if (filters.username) params.username = filters.username;
      if (filters.from) params.from = filters.from;
      if (pageParam) params.before = pageParam;
      return (await api.get('/admin/logins', { params })).data;
    },
    initialPageParam: 0,
    getNextPageParam: (last) => (last.more && last.entries.length ? last.entries[last.entries.length - 1].id : undefined),
    staleTime: 0,
  });
}

/** The owner sets how many days the record is kept. */
export function useSetLoginRetention() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (retentionDays) => (await api.put('/admin/logins/settings', { retentionDays })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'logins'] }),
  });
}

/** What the owner may tune without a restart (Sistema → Desempenho): the values in use, the defaults, the limits of this machine,
 *  and what the OCR service says it uses. Asked again every few seconds while the tab is open, so a change made elsewhere shows. */
export function usePerformance() {
  return useQuery({
    queryKey: ['admin', 'performance'],
    queryFn: async () => (await api.get('/admin/performance')).data,
    staleTime: 0,
    refetchInterval: 10000,
  });
}

/** The owner changes some of the values; a null goes back to the default of the installation. */
export function useSetPerformance() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (changes) => (await api.put('/admin/performance', changes)).data,
    onSuccess: (data) => queryClient.setQueryData(['admin', 'performance'], data),
  });
}
