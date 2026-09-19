import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const fetchStats = async () => {
  const { data } = await api.get('/stats');
  return data;
};

export const useStats = () => {
  return useQuery({
    queryKey: ['stats'],
    queryFn: fetchStats,
  });
};
