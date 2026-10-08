import { useQuery } from '@tanstack/react-query';
import { api } from '../../../lib/api';

/**
 * Where a work goes on in its series (#187): the official collection it is in and the work that follows it in its own group
 * (the next chapter of a chapter, the next volume of a volume). `{ collection: null, next: null }` for a work in no collection.
 * Under 'work', so that whatever refreshes the works refreshes this.
 */
export function useWorkSeries(id) {
  return useQuery({
    queryKey: ['work', id, 'series'],
    queryFn: async () => (await api.get(`/works/${id}/series`)).data,
    enabled: !!id,
  });
}
