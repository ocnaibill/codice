import { create } from 'zustand';

export const useGlobalStore = create((set) => ({
  // Estado da UI
  searchQuery: '',
  activeBookId: null, 
  books: [],

  // Ações
  isUploadModalOpen: false,
  setSearchQuery: (query) => set({ searchQuery: query }),
  openBook: (id) => set({ activeBookId: id }),
  closeBook: () => set({ activeBookId: null }),
  setBooks: (books) => set({ books }),
openUploadModal: () => set({ isUploadModalOpen: true }),
  closeUploadModal: () => set({ isUploadModalOpen: false }),
}));
