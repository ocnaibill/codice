import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { serverMessage } from '../../../lib/serverMessage';

const KEY = ['kindle'];

/** { email, available }: the address of this account's e-reader ('' for none) and whether the server sends books by e-mail. */
export function useKindle() {
  return useQuery({
    queryKey: KEY,
    queryFn: async () => (await api.get('/auth/kindle')).data,
    staleTime: 60_000,
    // The address belongs to one account: it is dropped with the screen, never shown to the next person on this browser.
    gcTime: 0,
  });
}

export function useSetKindleEmail() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (email) => (await api.put('/auth/kindle', { email })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

/** Mails a book to the e-reader; `fileId` picks the file, and absent the server chooses the best format the work has. */
export function useSendToKindle() {
  return useMutation({
    mutationFn: async ({ workId, fileId }) => (await api.post(`/works/${workId}/send-to-kindle`, fileId ? { fileId } : {})).data,
  });
}

/**
 * What to tell of a failed send. The server's own sentences for "the mail server is off" (503) and "the mail server refused" (502)
 * are written for the person and say what to do, so they are shown; any other failure of the server is not read out.
 */
export function sendFailure(error, fallback = 'Não foi possível enviar o livro.') {
  const response = error?.response;
  if ((response?.status === 502 || response?.status === 503) && typeof response.data === 'string' && response.data.trim()) {
    return response.data.trim();
  }
  return serverMessage(error, fallback);
}

/** The formats a Kindle takes by e-mail. */
export const KINDLE_FORMATS = ['epub', 'pdf', 'txt'];
