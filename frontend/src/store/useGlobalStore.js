import { create } from 'zustand';

export const useGlobalStore = create((set) => ({
  // UI State
  searchQuery: '',
  // The work whose sheet (edition, language and file choice) is open, if any.
  sheetWorkId: null,
  // What the reader has open: a work, one of its files, and whether to ignore the saved
  // position and start over (choosing a version starts from that version's own position, or
  // from the beginning if the person asks).
  activeBookId: null,
  activeFileId: null,
  fromStart: false,
  // A place to open at (a note's locator), and a counter so that asking for the same place
  // again in the same file reopens the viewer there.
  seek: null,
  books: [],

  // Actions
  isUploadModalOpen: false,
  setSearchQuery: (query) => set({ searchQuery: query }),
  openWork: (id) => set({ sheetWorkId: id, activeBookId: null, activeFileId: null, adminOpen: false }),
  closeSheet: () => set({ sheetWorkId: null }),
  openBook: (id, fileId = null, { fromStart = false, locator = null } = {}) =>
    set((state) => ({
      activeBookId: id,
      activeFileId: fileId,
      fromStart,
      seek: locator ? { locator, n: (state.seek?.n ?? 0) + 1 } : null,
      sheetWorkId: null,
    })),
  closeBook: () => set({ activeBookId: null, activeFileId: null, fromStart: false, seek: null, sheetWorkId: null, adminOpen: false }),
  adminOpen: false,
  openAdmin: () => set({ adminOpen: true, activeBookId: null, activeFileId: null, sheetWorkId: null }),
  setBooks: (books) => set({ books }),
  openUploadModal: () => set({ isUploadModalOpen: true }),
  closeUploadModal: () => set({ isUploadModalOpen: false }),
}));
