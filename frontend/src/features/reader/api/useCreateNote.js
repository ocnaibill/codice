import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

export const useCreateNote = (workId) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (quote) => api.post(`/works/${workId}/notes`, { quote }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notes'] });
    },
  });
};
