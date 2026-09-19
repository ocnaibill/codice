import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const fetchFavorites = async () => {
  const { data } = await api.get('/favorites');
  return data; // { data: [...], total }
};

export const useFavorites = () => {
  return useQuery({
    queryKey: ['favorites'],
    queryFn: fetchFavorites,
  });
};
