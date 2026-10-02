import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** Creates a concept from a name (a pending [[link]] of a note). The server links the notes that already cited it,
 *  so the notes are read again: the key starts with 'notes'. */
export const useCreateConcept = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name) => api.post('/concepts', { name }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notes'] }),
  });
};
