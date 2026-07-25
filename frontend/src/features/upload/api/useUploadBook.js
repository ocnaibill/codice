import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const uploadBook = async ({ file, onProgress }) => {
  // Use FormData for file uploads instead of JSON
  const formData = new FormData();
  formData.append('document', file);

  const { data } = await api.post('/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        const percentCompleted = Math.round((progressEvent.loaded * 100) / progressEvent.total);
        if (onProgress) onProgress(percentCompleted);
      }
    },
  });
  return data;
};

export const useUploadBook = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: uploadBook,
    onSuccess: () => {
      // Invalidate works cache upon upload success to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['works'] });
    },
  });
};