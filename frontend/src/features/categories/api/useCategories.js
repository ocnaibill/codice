import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

/** The tree of categories, flat and sorted by name, each with how many works are in it (and under it): `{ id, parentId, name, works, own }`. With `covers`, each also has the covers of a few of its works. */
export function useCategories({ covers = false } = {}) {
  return useQuery({
    queryKey: ['categories', { covers }],
    queryFn: async () => (await api.get('/categories', { params: covers ? { covers: 1 } : undefined })).data.data,
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

/** The rules that put works in categories (DEC-140): `{ id, categoryId, term }`, by category and term. Owner and admin. */
export function useCategoryRules() {
  return useQuery({
    queryKey: ['categoryRules'],
    queryFn: async () => (await api.get('/admin/categories/rules')).data.data,
    staleTime: 0,
  });
}

function useRuleAction(run) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['categoryRules'] });
      refreshLibrary(queryClient);
    },
  });
}

/** A term for a category: a work with a tag that is that term (no regard to case or accents) is put in it when the rules are applied. */
export const useAddRule = () => useRuleAction(({ categoryId, term }) => api.post(`/admin/categories/${categoryId}/rules`, { term }));
/** The category stops having a term; the works it already put in it stay. */
export const useRemoveRule = () => useRuleAction((ruleId) => api.delete(`/admin/categories/rules/${ruleId}`));

/** What applying the rules would do, without doing it: `{ categories: [{ id, name, matched, fresh }], links, works, withoutNow, withoutAfter }`. */
export function useRulesPreview() {
  return useMutation({ mutationFn: async () => (await api.get('/admin/categories/rules/preview')).data });
}
/** Applies the rules, if the preview that was looked at (`links` new places) is still what they would do. Answers `{ links, works }`. */
export const useApplyRules = () => useRuleAction(async (links) => (await api.post('/admin/categories/rules/apply', { links })).data);

/** The list of categories and terms that is offered to start from. It is only read when `enabled`: when someone asks to see it. */
export function useStarterList({ enabled }) {
  return useQuery({
    queryKey: ['categoryStarter'],
    queryFn: async () => (await api.get('/admin/categories/starter')).data.data,
    enabled,
    staleTime: Infinity,
  });
}
/** Makes the chosen categories of the list, with their subcategories and terms (nothing is put in any category). Answers `{ categories, rules }`. */
export const useCreateFromStarter = () => useRuleAction(async (groups) => (await api.post('/admin/categories/starter', { groups })).data);
