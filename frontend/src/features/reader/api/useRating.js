import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * The stars the person gives a work (DEC-154): `mutate(4)` gives four, `mutate(0)` takes them away. The page of the work is asked again.
 */
export const useRating = (workId) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (stars) => (stars > 0 ? api.put(`/works/${workId}/rating`, { stars }) : api.delete(`/works/${workId}/rating`)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['work', workId] }),
  });
};
