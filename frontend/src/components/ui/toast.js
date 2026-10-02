import { create } from 'zustand';

/** How long a notice stays, by tone, in ms. An error stays longer: it has to be read, and often acted on. */
export const DURATIONS = { success: 5000, info: 5000, warning: 7000, error: 9000 };
/** At most this many at once: a new one pushes the oldest out, so a burst of uploads does not wall the screen. */
export const MAX_TOASTS = 3;

let seq = 0;

/**
 * The notices on screen (#77): one place for "metadados atualizados", "não foi possível…" and the like, in the place
 * of the banners, loose red texts and `alert` calls that each screen made its own. A notice with a `key` replaces
 * the one on screen with the same key instead of piling up.
 */
export const useToasts = create((set) => ({
  items: [],
  show: (toast) => {
    const id = ++seq;
    const tone = toast.tone ?? 'info';
    const item = { id, tone, title: toast.title, message: toast.message, action: toast.action, key: toast.key, duration: toast.duration ?? DURATIONS[tone] };
    set((state) => {
      const rest = toast.key ? state.items.filter((t) => t.key !== toast.key) : state.items;
      return { items: [...rest, item].slice(-MAX_TOASTS) };
    });
    return id;
  },
  dismiss: (id) => set((state) => ({ items: state.items.filter((t) => t.id !== id) })),
  clear: () => set({ items: [] }),
}));

const show = (tone) => (title, options = {}) => useToasts.getState().show({ ...options, tone, title });

/** toast.success("Salvo"), toast.error("Não foi possível salvar", { message: "…", action: { label, onClick } }). */
export const toast = {
  success: show('success'),
  info: show('info'),
  warning: show('warning'),
  error: show('error'),
  dismiss: (id) => useToasts.getState().dismiss(id),
};
