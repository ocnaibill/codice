import { useQuery } from '@tanstack/react-query';
import { api } from '../../lib/api';

/** What this installation says about itself (DEC-120): { version, license, licenseUrl, sourceUrl }. Nothing else is in it. */
export function useAbout() {
  return useQuery({
    queryKey: ['about'],
    queryFn: async () => (await api.get('/about')).data,
    // It only changes when the server is replaced.
    staleTime: Infinity,
  });
}
