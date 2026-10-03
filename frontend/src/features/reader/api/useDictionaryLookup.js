import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** What the dictionaries have of a word of a language (#109), with the definitions in the language the reader prefers first.
 *  The word goes to the server of the library and no further; an answer is kept for a few minutes, so that selecting the same
 *  word again, or changing the language and back, asks nothing. */
export const useDictionaryLookup = ({ word, lang, prefer }) =>
  useQuery({
    queryKey: ['dictionary', lang, prefer ?? '', word],
    queryFn: async () => (await api.get('/dictionary', { params: { word, lang, ...(prefer ? { prefer } : {}) } })).data,
    enabled: !!word && !!lang,
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
