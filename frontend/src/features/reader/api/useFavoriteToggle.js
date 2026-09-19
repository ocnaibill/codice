import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

export const useFavoriteToggle = (workId) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (nextIsFavorite) =>
      nextIsFavorite
        ? api.post(`/works/${workId}/favorite`)
        : api.delete(`/works/${workId}/favorite`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['work', workId] });
      queryClient.invalidateQueries({ queryKey: ['favorites'] });
      queryClient.invalidateQueries({ queryKey: ['works'] });
    },
  });
};
