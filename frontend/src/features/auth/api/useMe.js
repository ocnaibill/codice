import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

export const isStaff = (me) => me?.role === 'owner' || me?.role === 'admin';

/** Who is signed in and with which role; the server is the authority on the role. */
export function useMe(enabled = true) {
  return useQuery({
    queryKey: ['me'],
    queryFn: async () => (await api.get('/auth/me')).data,
    enabled,
    staleTime: 60_000,
  });
}
