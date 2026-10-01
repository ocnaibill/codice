import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const KEY = ['app-tokens'];

/** The accesses this account gave to reading apps (DEC-071): [{ id, name, createdAt, lastUsedAt? }], newest first. */
export function useAppTokens() {
  return useQuery({
    queryKey: KEY,
    queryFn: async () => (await api.get('/auth/app-tokens')).data,
    // Belongs to one account: it is dropped as soon as the screen closes, so it can never be shown to the next
    // person who signs in on this browser.
    gcTime: 0,
  });
}

/** Creates one; the answer carries the token, the only time it is ever shown. */
export function useCreateAppToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name) => (await api.post('/auth/app-tokens', { name })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

export function useRevokeAppToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id) => api.delete(`/auth/app-tokens/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

/** What an OPDS app is told to connect to: this same address, so it is the one the person already uses. */
export function catalogAddress(location = window.location) {
  return `${location.origin}/opds/v1.2/catalog`;
}

/** An address that only works on the machine that typed it: a phone or an e-reader cannot reach it. */
export function isLocalAddress(location = window.location) {
  const host = location.hostname.replace(/^\[|\]$/g, '');
  return host === 'localhost' || host.endsWith('.localhost') || host === '::1' || /^127(\.\d{1,3}){3}$/.test(host);
}
