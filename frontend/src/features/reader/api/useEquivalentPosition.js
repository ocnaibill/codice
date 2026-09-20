import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * Looks, in the file being opened, for the place that answers to the position the caller has in
 * another version of the same work (RF-042). Runs once per pair of files: the answer does not
 * change while the reader stays open, and asking again buys nothing.
 */
export function useEquivalentPosition(destFileId, sourceFileId) {
  return useQuery({
    queryKey: ['equivalent-position', destFileId, sourceFileId],
    queryFn: async () => (await api.get(`/progress/files/${destFileId}/equivalent`, { params: { from: sourceFileId } })).data,
    enabled: !!destFileId && !!sourceFileId && destFileId !== sourceFileId,
    staleTime: Infinity,
    retry: false,
  });
}

/** Records that a suggested position was accepted (history only: opening it is a normal seek). */
export function useAcceptEquivalentPosition(destFileId) {
  return useMutation({
    mutationFn: ({ sourceFileId, locator, method, confidence, precision }) =>
      api.post(`/progress/files/${destFileId}/equivalent/accept`, { sourceFileId, locator, method, confidence, precision }),
  });
}
