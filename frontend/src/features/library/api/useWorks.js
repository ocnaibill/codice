import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api'; // Ajuste o caminho relativo se necessário

// A função fetcher pura
const fetchWorks = async () => {
  const { data } = await api.get('/works');
  return data;
};

// O Hook que o componente vai consumir
export const useWorks = () => {
  return useQuery({
    queryKey: ['works'],
    queryFn: fetchWorks,
  });
};