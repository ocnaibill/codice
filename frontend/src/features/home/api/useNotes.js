import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const fetchNotes = async ({ queryKey }) => {
  const [, { limit }] = queryKey;
  const { data } = await api.get(`/notes?limit=${limit}`);
  return data; // { data: [...] }
};

export const useNotes = ({ limit = 4 } = {}) => {
  return useQuery({
    queryKey: ['notes', { limit }],
    queryFn: fetchNotes,
  });
};
