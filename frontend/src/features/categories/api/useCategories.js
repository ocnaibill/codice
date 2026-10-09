import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/** The tree of categories, flat and sorted by name, each with how many works are in it (and under it): `{ id, parentId, name, works, own }`. */
export function useCategories() {
  return useQuery({
    queryKey: ['categories'],
    queryFn: async () => (await api.get('/categories')).data.data,
    staleTime: 30_000,
  });
}

function useCategoryAction(run) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => refreshLibrary(queryClient),
  });
}

/** A new category, at the top (`parentId` null) or under another. Owner and admin. */
export const useCreateCategory = () => useCategoryAction(({ name, parentId }) => api.post('/admin/categories', { name, parentId }));
/** The new name and the new place of a category. */
export const useUpdateCategory = () => useCategoryAction(({ id, name, parentId }) => api.put(`/admin/categories/${id}`, { name, parentId }));
/** Deletes a category with no subcategories; the works that were in it only stop being. */
export const useDeleteCategory = () => useCategoryAction((id) => api.delete(`/admin/categories/${id}`));
/** The categories a work is in, from now on: the ones not named stop being. */
export const useSetWorkCategories = (workId) => useCategoryAction((ids) => api.put(`/works/${workId}/categories`, { ids }));
