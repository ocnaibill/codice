import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const fetchNotes = async ({ queryKey }) => {
  const [, { limit, sample }] = queryKey;
  const { data } = await api.get(`/notes?limit=${limit}${sample ? '&sample=true' : ''}`);
  return data; // { data: [...] }
};

/**
 * The notes of the person: the newest, or with `sample` a few at random from all they kept (the home shows some of what was written, and
 * another few each visit). A sample is never fresh, so it is drawn again every time the page comes to it, and only then: coming back to the
 * window does not shuffle what is on the page.
 */
export const useNotes = ({ limit = 4, sample = false } = {}) => {
  return useQuery({
    queryKey: ['notes', { limit, sample }],
    queryFn: fetchNotes,
    ...(sample ? { staleTime: 0, refetchOnWindowFocus: false } : {}),
  });
};
