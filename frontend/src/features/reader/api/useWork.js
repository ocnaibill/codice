import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const fetchWork = async (id) => {
  const { data } = await api.get(`/works/${id}`);
  return data;
};

// `fresh` makes every opening ask the server again: the sheet shows where the person is in each
// file, which another tab or device may have moved since the last look.
export const useWork = (id, { fresh = false } = {}) => {
  return useQuery({
    queryKey: ['work', id],
    queryFn: () => fetchWork(id),
    enabled: !!id,
    ...(fresh ? { staleTime: 0 } : {}),
  });
};
