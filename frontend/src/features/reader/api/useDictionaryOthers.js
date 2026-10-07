import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** The word in the other languages the installed dictionaries have it in (#183): asked when the language of the book has nothing of it.
 *  The answer is the languages that have it, in order, each with what a lookup in it finds. */
export const useDictionaryOthers = ({ word, lang, prefer, enabled = true }) =>
  useQuery({
    queryKey: ['dictionary-others', lang, prefer ?? '', word],
    queryFn: async () => (await api.get('/dictionary/others', { params: { word, lang, ...(prefer ? { prefer } : {}) } })).data,
    enabled: enabled && !!word && !!lang,
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
