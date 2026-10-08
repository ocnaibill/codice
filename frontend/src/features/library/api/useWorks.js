import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

// Fetcher function with pagination, search, and filter support
const fetchWorks = async ({ queryKey }) => {
  const [_key, { page, limit, search, inProgress, favorite, formatGroup, sort, person, role }] = queryKey;
  const params = new URLSearchParams();
  if (page) params.set('page', page);
  if (limit) params.set('limit', limit);
  if (search) params.set('search', search);
  if (inProgress) params.set('inProgress', 'true');
  if (favorite) params.set('favorite', 'true');
  if (formatGroup && formatGroup !== 'all') params.set('formatGroup', formatGroup);
  if (sort && sort !== 'added') params.set('sort', sort);
  if (person) params.set('person', person);
  if (role) params.set('role', role);
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
  sort = 'added',
  person,
  role,
  enabled = true,
} = {}) => {
  return useQuery({
    queryKey: ['works', { page, limit, search, inProgress, favorite, formatGroup, sort, person, role }],
    queryFn: fetchWorks,
    enabled,
    // Redis/WebSocket notifications are best effort. Pending work must still
    // reach its final state in the UI if a notification is lost.
    refetchInterval: (query) => query.state.data?.data?.some(
      (work) => ['UNKNOWN', 'QUEUED', 'ANALYZING'].includes(work.mediaStatus)
    ) ? 3000 : false,
  });
};
