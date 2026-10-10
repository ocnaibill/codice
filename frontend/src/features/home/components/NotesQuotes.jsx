import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { downloadFile } from '../../../lib/download';
import { useGlobalStore } from '../../../store/useGlobalStore';

/**
 * What the person wrote in the margins, as the home shows it: two of the notes, drawn again each time the page is read (the caller asks for a
 * sample), and, when they kept more, the way to all of them.
 */
export function NotesQuotes({ notes, total = 0, isLoading }) {
  const openNotes = useGlobalStore((state) => state.openNotes);
  return (
    <div className="library-panel">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 pb-4">
        <h3 className="font-display text-xl text-ink">À margem da leitura</h3>
        {!isLoading && notes.length > 0 && (
          <div className="flex items-center gap-3">
            {total > notes.length && (
              <button type="button" onClick={openNotes} className="font-body text-[11px] tracking-[0.44px] text-brand hover:underline">
                Ver todas ({total})
              </button>
            )}
            <button
              onClick={() => downloadFile('/notes/export?format=md', 'codice-anotacoes.md').catch(() => {})}
              className="font-body text-[11px] tracking-[0.44px] text-brand hover:underline"
              title="Baixa todas as suas anotações, com a referência de cada livro"
            >
              Exportar tudo
            </button>
          </div>
        )}
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : notes.length === 0 ? (
        <EmptyState>Nenhuma anotação ainda. Use o botão "Notas" no leitor para guardar trechos e ideias.</EmptyState>
      ) : (
        <div className="home-notes-list">
          {notes.map((note) => (
            <blockquote key={note.id} className="rounded-l-sm bg-gradient-to-r from-brand/10 to-transparent py-1 pl-3">
              <p className="font-display text-xl italic leading-[29px] tracking-[-0.12px] text-ink">
                {note.quote ? `“${note.quote}”` : note.body}
              </p>
              <p className="mt-1 text-right font-body text-[11px] italic tracking-[-0.12px] text-ink">
                — {note.workTitle}, {note.workAuthor}
                {note.sourceAvailable === false && <span className="not-italic text-ink-faint"> (fonte indisponível)</span>}
              </p>
            </blockquote>
          ))}
        </div>
      )}
    </div>
  );
}
