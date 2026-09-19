import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';

/** The transfer of ownership waiting for an answer that involves the signed-in account. */
export const useOwnershipTransfer = (enabled = true) =>
  useQuery({
    queryKey: ['ownership'],
    queryFn: async () => (await api.get('/ownership/transfer')).data,
    enabled,
    staleTime: 30_000,
  });

function useOwnershipAction(run) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => {
      // Roles may have changed: refresh who we are, the transfer and the accounts.
      for (const key of ['ownership', 'me', 'admin']) queryClient.invalidateQueries({ queryKey: [key] });
    },
  });
}

export const useStartTransfer = () =>
  useOwnershipAction(async ({ targetId, formerRole, password }) =>
    (await api.post('/ownership/transfer', { targetId, formerRole, password })).data);
export const useCancelTransfer = () => useOwnershipAction(() => api.delete('/ownership/transfer'));
export const useAcceptTransfer = () => useOwnershipAction((password) => api.post('/ownership/transfer/accept', { password }));
export const useDeclineTransfer = () => useOwnershipAction(() => api.post('/ownership/transfer/decline'));

export function useAckNotice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id) => api.post(`/auth/notices/${id}/ack`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['me'] }),
  });
}
