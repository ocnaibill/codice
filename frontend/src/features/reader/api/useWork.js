import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const fetchWork = async (id) => {
  const { data } = await api.get(`/works/${id}`);
  return data;
};

export const useWork = (id) => {
  return useQuery({
    queryKey: ['work', id],
    queryFn: () => fetchWork(id),
    enabled: !!id,
  });
};
