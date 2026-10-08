import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

// Who is credited on a work (#185); for owner and admin. What shows the authors on screen (the cards, the sheet) is refreshed.
function useRefreshing() {
  const queryClient = useQueryClient();
  return () => refreshLibrary(queryClient);
}

/** Credits a person on the work, after the others with the same role. */
export function useAddContributor(workId) {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: async ({ name, role }) => (await api.post(`/works/${workId}/contributors`, { name, role })).data,
    onSuccess: refresh,
  });
}

/** The person is not credited with that role on the work any more. */
export function useRemoveContributor(workId) {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ personId, role }) => api.delete(`/works/${workId}/contributors/${personId}/${role}`),
    onSuccess: refresh,
  });
}

/** The people of a role get their places in the order of `personIds`, which must be all of them. */
export function useOrderContributors(workId) {
  const refresh = useRefreshing();
  return useMutation({
    mutationFn: ({ role, personIds }) => api.put(`/works/${workId}/contributors/order`, { role, personIds }),
    onSuccess: refresh,
  });
}
