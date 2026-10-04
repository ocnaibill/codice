import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const KEY = ['sessions'];

/** The live sessions of this account (DEC-070): [{ id, userAgent, ip?, createdAt, lastSeenAt, expiresAt, current }], the
 *  one most recently used first. Belongs to one account, so it is dropped as soon as the screen closes. */
export function useSessions() {
  return useQuery({
    queryKey: KEY,
    queryFn: async () => (await api.get('/auth/sessions')).data,
    gcTime: 0,
    staleTime: 0,
  });
}

/** Ends one session of this account. The one in use is not offered by the screen: signing out is the menu's. */
export function useRevokeSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id) => api.delete(`/auth/sessions/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

/** Ends every session of this account but the one in use; answers { revoked }. */
export function useRevokeOtherSessions() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post('/auth/sessions/revoke-others')).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

/** The sessions of ANOTHER account, for the staff: the same, with no address. */
export function useUserSessions(userId) {
  return useQuery({
    queryKey: ['user-sessions', userId],
    queryFn: async () => (await api.get(`/users/${userId}/sessions`)).data,
    gcTime: 0,
    staleTime: 0,
  });
}

export function useRevokeUserSession(userId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (sessionId) => api.delete(`/users/${userId}/sessions/${sessionId}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['user-sessions', userId] }),
  });
}

export function useRevokeAllUserSessions(userId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.delete(`/users/${userId}/sessions`)).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['user-sessions', userId] }),
  });
}
