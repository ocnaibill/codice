import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * Turns an upload error into something a person can act on. The server answers
 * 409 for bytes it already holds (with the existing record), 415 for a file
 * whose content is not the format it claims, and 413 for one that is too large.
 */
export function describeUploadError(error) {
  const res = error?.response;
  if (!res) {
    return error?.code === 'ECONNABORTED' ? 'The upload timed out.' : 'Could not reach the server.';
  }
  if (res.status === 409 && res.data && typeof res.data === 'object') {
    const state = res.data.retired ? ' (retired from the library)' : '';
    return `Already in the library as "${res.data.title}"${state}.`;
  }
  if (res.status === 413) return 'The file is too large.';
  if (res.status === 415) return typeof res.data === 'string' ? res.data.trim() : 'The file is not valid for its format.';
  if (res.status === 403) return 'Only administrators can add books.';
  if (res.status === 400 && typeof res.data === 'string') return res.data.trim();
  return 'Error uploading file.';
}

const uploadBook = async ({ file, onProgress }) => {
  // Use FormData for file uploads instead of JSON
  const formData = new FormData();
  formData.append('document', file);

  const { data } = await api.post('/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    // Large files take longer than the default 10 s to send.
    timeout: 0,
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