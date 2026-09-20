import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { downloadFile } from '../../../lib/download';

export function NotesQuotes({ notes, isLoading }) {
  return (
    <div className="w-full rounded-lg bg-white p-4 shadow-[0px_1px_2px_rgba(0,0,0,0.06)]">
      <div className="flex items-center justify-between pb-4">
        <h3 className="font-display text-xs uppercase tracking-[0.55px] text-[#402e25]">Algumas de suas anotações e frases</h3>
        {!isLoading && notes.length > 0 && (
          <button
            onClick={() => downloadFile('/notes/export?format=md', 'codice-anotacoes.md').catch(() => {})}
            className="font-body text-[11px] tracking-[0.44px] text-brand hover:underline"
            title="Baixa todas as suas anotações, com a referência de cada livro"
          >
            Exportar tudo
          </button>
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
        <div className="flex flex-col gap-6">
          {notes.map((note) => (
            <blockquote key={note.id} className="rounded-l-sm bg-gradient-to-r from-brand/10 to-transparent py-1 pl-3">
              <p className="font-body text-xl italic leading-[29px] tracking-[-0.12px] text-ink">
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
