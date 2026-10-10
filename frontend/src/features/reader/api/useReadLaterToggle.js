import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * Puts a work in the list "Ler depois" of the person, or takes it out (DEC-152). The list is made by the server the first time; the page of
 * the work and the lists of the person are asked again.
 */
export const useReadLaterToggle = (workId) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (next) => (next ? api.put(`/works/${workId}/read-later`) : api.delete(`/works/${workId}/read-later`)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['work', workId] });
      queryClient.invalidateQueries({ queryKey: ['collections'] });
      queryClient.invalidateQueries({ queryKey: ['collection'] });
    },
  });
};
