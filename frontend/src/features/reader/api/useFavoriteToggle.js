import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

export const useFavoriteToggle = (workId) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (nextIsFavorite) =>
      nextIsFavorite
        ? api.post(`/works/${workId}/favorite`)
        : api.delete(`/works/${workId}/favorite`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['work', workId] });
      refreshLibrary(queryClient);
    },
  });
};
