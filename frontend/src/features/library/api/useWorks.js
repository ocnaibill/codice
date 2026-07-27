import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

// Fetcher function with pagination and search support
const fetchWorks = async ({ queryKey }) => {
  const [_key, { page, limit, search }] = queryKey;
  const params = new URLSearchParams();
  if (page) params.set('page', page);
  if (limit) params.set('limit', limit);
  if (search) params.set('search', search);
  const { data } = await api.get(`/works?${params.toString()}`);
  return data; // Returns { data: [...], total, page, limit, totalPages }
};

// React Query hook consumed by components
export const useWorks = ({ page = 1, limit = 50, search = '' } = {}) => {
  return useQuery({
    queryKey: ['works', { page, limit, search }],
    queryFn: fetchWorks,
    placeholderData: (previousData) => previousData, // Keep previous data while fetching next page
  });
};