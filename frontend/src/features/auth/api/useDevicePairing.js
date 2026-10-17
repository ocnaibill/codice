import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** The code as it is shown ("BCDF-2345"): capitals, in two groups of four. Empty or shorter is left as it is typed. */
export function formatPairCode(typed) {
  const clean = String(typed || '').toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 8);
  return clean.length > 4 ? `${clean.slice(0, 4)}-${clean.slice(4)}` : clean;
}

/** Whether what was typed can be a code (eight symbols). */
export function isPairCode(typed) {
  return String(typed || '').toUpperCase().replace(/[^A-Z0-9]/g, '').length === 8;
}

/** Looks a request up by its code: { userCode, name, platform, client, ip, requestedAt, expiresAt }. A mutation, not a cache: a code is looked up once. */
export function useLookupPairing() {
  return useMutation({ mutationFn: async (code) => (await api.get(`/auth/device/${encodeURIComponent(code)}`)).data });
}

/** Connects the device to this account; `name` is what the person wants it called (empty keeps the one the device suggested). */
export function useApprovePairing() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ code, name }) => (await api.post(`/auth/device/${encodeURIComponent(code)}/approve`, { name })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['app-tokens'] }),
  });
}

export function useDenyPairing() {
  return useMutation({ mutationFn: async (code) => (await api.post(`/auth/device/${encodeURIComponent(code)}/deny`)).data });
}

/** Whether this instance lets readers be connected by code (the owner's switch). */
export function usePairingEnabled(enabled = true) {
  return useQuery({
    queryKey: ['admin', 'device-pairing'],
    queryFn: async () => (await api.get('/admin/device-pairing')).data,
    enabled,
  });
}

export function useSetPairingEnabled() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (enabled) => (await api.put('/admin/device-pairing', { enabled })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'device-pairing'] }),
  });
}

/** What any signed-in person may know: whether this instance connects readers by code (so the button is shown only when it works). */
export function useDevicePairingInfo() {
  return useQuery({
    queryKey: ['device-info'],
    queryFn: async () => (await api.get('/device/info')).data,
    staleTime: 60_000,
  });
}
