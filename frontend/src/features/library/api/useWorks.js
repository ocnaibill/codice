import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

// Fetcher function with pagination, search, and filter support
const fetchWorks = async ({ queryKey }) => {
  const [_key, { page, limit, search, inProgress, favorite, formatGroup }] = queryKey;
  const params = new URLSearchParams();
  if (page) params.set('page', page);
  if (limit) params.set('limit', limit);
  if (search) params.set('search', search);
  if (inProgress) params.set('inProgress', 'true');
  if (favorite) params.set('favorite', 'true');
  if (formatGroup && formatGroup !== 'all') params.set('formatGroup', formatGroup);
  const { data } = await api.get(`/works?${params.toString()}`);
  return data; // Returns { data: [...], total, page, limit, totalPages }
};

// React Query hook consumed by components
export const useWorks = ({
  page = 1,
  limit = 50,
  search = '',
  inProgress = false,
  favorite = false,
  formatGroup = 'all',
} = {}) => {
  return useQuery({
    queryKey: ['works', { page, limit, search, inProgress, favorite, formatGroup }],
    queryFn: fetchWorks,
    placeholderData: (previousData) => previousData, // Keep previous data while fetching next page
  });
};
