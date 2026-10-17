import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

const form = (file, mapping) => {
  const data = new FormData();
  data.append('file', file);
  if (mapping) data.append('mapping', JSON.stringify(mapping));
  return data;
};

/** What a file of notes from a reading device holds (a Kindle's My Clippings.txt, a KOReader export in JSON): its books, and the works of the library each may be. Writes nothing. */
export function usePreviewImport() {
  return useMutation({
    mutationFn: async (file) => (await api.post('/notes/import/preview', form(file))).data,
  });
}

/** Writes the notes of the books that were matched to a work: `mapping` is the work of each book, by the key the preview gave it. */
export function useImportNotes() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ file, mapping }) => (await api.post('/notes/import', form(file, mapping))).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notes'] }),
  });
}

/** The work chosen for each book to begin with: the one the server was sure of (a book with no work is left out). */
export function initialMapping(groups) {
  return Object.fromEntries((groups ?? []).map((g) => [g.key, g.suggested || 0]));
}

/** The mapping the server takes: only the books that have a work. */
export function chosenMapping(mapping) {
  return Object.fromEntries(Object.entries(mapping).filter(([, workId]) => workId > 0));
}

const SKIPPED = {
  bookmarks: 'marcadores, que não têm texto',
  empty: 'entradas sem texto',
  limit: 'trechos que o Kindle não deixou copiar (limite de recortes do livro)',
  repeated: 'destaques repetidos',
};

/** What was left out of the file, in words; empty when nothing was. */
export function skippedText(skipped) {
  const parts = Object.entries(skipped ?? {})
    .filter(([, n]) => n > 0)
    .map(([reason, n]) => `${n} ${SKIPPED[reason] ?? reason}`);
  return parts.length ? `Ficou de fora: ${parts.join('; ')}.` : '';
}
