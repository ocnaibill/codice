import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/** The page of a person (#186): the names they go by, the roles they have on works of the library, and the collections those are in. */
export function usePerson(id) {
  return useQuery({
    queryKey: ['person', id],
    queryFn: async () => (await api.get(`/people/${id}`)).data,
    enabled: !!id,
  });
}
