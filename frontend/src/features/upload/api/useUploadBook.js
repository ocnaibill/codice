import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const uploadBook = async (file) => {
  // Para envio de arquivos, precisamos usar FormData em vez de JSON
  const formData = new FormData();
  formData.append('document', file);

  const { data } = await api.post('/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
  return data;
};

export const useUploadBook = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: uploadBook,
    onSuccess: () => {
      // Assim que o upload terminar, invalidamos o cache da estante.
      // Isso faz o React Query buscar a lista nova automaticamente
      queryClient.invalidateQueries({ queryKey: ['works'] });
    },
  });
};