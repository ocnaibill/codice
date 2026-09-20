import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

// The person's own marginalia in one work. The key starts with 'notes' so that saving here
// also refreshes the short list on the home page.
export const useWorkNotes = (workId) =>
  useQuery({
    queryKey: ['notes', 'work', workId],
    queryFn: async () => (await api.get('/notes', { params: { workId, limit: 200 } })).data,
    enabled: !!workId,
    staleTime: 0,
  });

function useNotesMutation(mutationFn) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notes'] }),
  });
}

/** Saves a note. Given only text, it is a saved quotation (what the first version did). */
export const useCreateNote = (workId) =>
  useNotesMutation((input) => api.post(`/works/${workId}/notes`, typeof input === 'string' ? { quote: input } : input));

export const useUpdateNote = () => useNotesMutation(({ id, ...changes }) => api.patch(`/notes/${id}`, changes));

export const useDeleteNote = () => useNotesMutation((id) => api.delete(`/notes/${id}`));
