import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** The languages the installed dictionaries can be asked in (#109): the ones a word can be looked up in, and the ones the
 *  definitions can be had in. It is asked each time a card opens, so that a dictionary the owner has just installed (or taken
 *  away) is there (or not) at once. */
export const useDictionaryLanguages = () =>
  useQuery({
    queryKey: ['dictionary', 'languages'],
    queryFn: async () => (await api.get('/dictionary/languages')).data,
    staleTime: 0,
    retry: false,
  });
