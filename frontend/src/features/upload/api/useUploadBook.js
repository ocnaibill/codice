import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/**
 * Turns an upload error into something a person can act on. The server answers
 * 409 for bytes it already holds (with the existing record), 415 for a file
 * whose content is not the format it claims, and 413 for one that is too large.
 */
export function describeUploadError(error) {
  const res = error?.response;
  if (!res) {
    return error?.code === 'ECONNABORTED' ? 'O envio demorou demais e foi interrompido.' : 'Não foi possível falar com o servidor.';
  }
  if (res.status === 409 && res.data?.error === 'in_series') return `Já existe na série como “${res.data.title}”: foi pulado.`;
  if (res.status === 409 && res.data && typeof res.data === 'object') {
    const state = res.data.retired ? ' (na lixeira)' : '';
    return `Esse arquivo já está no acervo como “${res.data.title}”${state}.`;
  }
  if (res.status === 413) return 'O arquivo é grande demais.';
  if (res.status === 415) return typeof res.data === 'string' ? res.data.trim() : 'O conteúdo do arquivo não é válido para o formato dele.';
  if (res.status === 403) return 'Só quem administra o acervo pode adicionar arquivos.';
  if (res.status === 400 && typeof res.data === 'string') return res.data.trim();
  return 'Não foi possível enviar o arquivo.';
}

/** Whether the server said a file's number is already taken in its series (it is skipped, not an error). */
export const isInSeries = (error) => error?.response?.status === 409 && error.response.data?.error === 'in_series';

// `place` is where the work belongs (the bulk upload of a series, DEC-200): the server reads these fields only before the file.
export const uploadBook = async ({ file, onProgress, place }) => {
  // Use FormData for file uploads instead of JSON
  const formData = new FormData();
  for (const [field, value] of Object.entries(place ?? {})) {
    if (value !== undefined && value !== null && value !== '') formData.append(field, String(value));
  }
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
      refreshLibrary(queryClient);
    },
  });
};