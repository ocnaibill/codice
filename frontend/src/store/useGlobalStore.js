import { create } from 'zustand';

export const useGlobalStore = create((set) => ({
  // UI State
  searchQuery: '',
  libraryView: 'all',
  libraryPage: 1,
  librarySort: 'added', // 'added' (newest first), 'title' or 'author' (as the account shows names, #64)
  libraryViewMode: 'grid',
  setLibraryView: (libraryView) => set({ libraryView, libraryPage: 1, searchQuery: '', adminOpen: false, notesOpen: false, activeBookId: null, activeFileId: null, sheetWorkId: null, collectionSheetId: null, personSheetId: null }),
  setLibraryPage: (libraryPage) => set({ libraryPage }),
  setLibrarySort: (librarySort) => set({ librarySort, libraryPage: 1 }),
  setLibraryViewMode: (libraryViewMode) => set({ libraryViewMode }),
  // The work whose sheet (edition, language and file choice) is open, if any.
  sheetWorkId: null,
  // The collection whose page is open, if any (#206). The sheet of a work opens over it, and closing that one comes back here.
  collectionSheetId: null,
  // The person whose page is open, if any (#186). It opens from the sheet of a work, which it replaces; the sheet of a work opens over it.
  personSheetId: null,
  openPerson: (id) => set({ personSheetId: id, sheetWorkId: null }),
  closePerson: () => set({ personSheetId: null }),
  openCollection: (id) => set({ collectionSheetId: id }),
  closeCollection: () => set({ collectionSheetId: null }),
  // The work whose metadata and suggestions are open for owner/admin (#70), and which part ('suggestions' or 'edit').
  metadataWorkId: null,
  metadataTab: 'suggestions',
  openMetadata: (id, tab = 'suggestions') => set({ metadataWorkId: id, metadataTab: tab }),
  closeMetadata: () => set({ metadataWorkId: null }),
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
  setSearchQuery: (query) => set({ searchQuery: query, ...(query.trim() ? { adminOpen: false, notesOpen: false } : {}) }),
  // The sheet of a work opens over whatever is on screen, the notes included.
  openWork: (id) => set({ sheetWorkId: id, activeBookId: null, activeFileId: null, adminOpen: false }),
  closeSheet: () => set({ sheetWorkId: null }),
  // `context` says what asked for the place ({ quote } of a note, or of a search hit), for when the place cannot be
  // opened and the reader has to say what it was.
  openBook: (id, fileId = null, { fromStart = false, locator = null, context = null } = {}) =>
    set((state) => ({
      activeBookId: id,
      activeFileId: fileId,
      fromStart,
      seek: locator ? { locator, context, n: (state.seek?.n ?? 0) + 1 } : null,
      sheetWorkId: null,
      collectionSheetId: null,
      personSheetId: null,
    })),
  // The place asked for could not be opened: open at the saved position instead.
  clearSeek: () => set({ seek: null }),
  // Closing the book goes back to what it was opened from, the screen of every note included.
  closeBook: () => set({ activeBookId: null, activeFileId: null, fromStart: false, seek: null, sheetWorkId: null, adminOpen: false }),
  adminOpen: false,
  // The tab of the administration that is open (#182): in the store so that the address can say it.
  adminTab: 'jobs',
  setAdminTab: (adminTab) => set({ adminTab }),
  openAdmin: () => set({ adminOpen: true, adminTab: 'jobs', notesOpen: false, activeBookId: null, activeFileId: null, sheetWorkId: null }),
  // The dialogs of the account that are open ('senha', 'preferencias', 'aplicativos', 'sobre', 'sessoes'), the last over the others (#182):
  // they are in the store so that the address says them, and the back button closes them.
  accountDialogs: [],
  openAccountDialog: (name) => set((state) => (state.accountDialogs.includes(name) ? state : { accountDialogs: [...state.accountDialogs, name] })),
  closeAccountDialog: (name) => set((state) => (state.accountDialogs.includes(name) ? { accountDialogs: state.accountDialogs.filter((open) => open !== name) } : state)),
  // All of the person's own notes, highlights and bookmarks (#13).
  notesOpen: false,
  openNotes: () => set({ notesOpen: true, adminOpen: false, searchQuery: '', activeBookId: null, activeFileId: null, sheetWorkId: null }),
  setBooks: (books) => set({ books }),
  openUploadModal: () => set({ isUploadModalOpen: true }),
  closeUploadModal: () => set({ isUploadModalOpen: false }),
}));
