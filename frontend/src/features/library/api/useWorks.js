import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

// Pure fetcher function
const fetchWorks = async () => {
  const { data } = await api.get('/works');
  return data;
};

// React Query hook consumed by components
export const useWorks = () => {
  return useQuery({
    queryKey: ['works'],
    queryFn: fetchWorks,
  });
};