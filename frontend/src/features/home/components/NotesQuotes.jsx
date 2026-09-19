import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';

export function NotesQuotes({ notes, isLoading }) {
  return (
    <div className="w-full rounded-lg bg-white p-4 shadow-[0px_1px_2px_rgba(0,0,0,0.06)]">
      <h3 className="pb-4 font-display text-xs uppercase tracking-[0.55px] text-[#402e25]">
        Algumas de suas anotações e frases
      </h3>

      {isLoading ? (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : notes.length === 0 ? (
        <EmptyState>Nenhuma citação salva ainda. Use o botão "+ Nota" no leitor para guardar trechos.</EmptyState>
      ) : (
        <div className="flex flex-col gap-6">
          {notes.map((note) => (
            <blockquote key={note.id} className="rounded-l-sm bg-gradient-to-r from-brand/10 to-transparent py-1 pl-3">
              <p className="font-body text-xl italic leading-[29px] tracking-[-0.12px] text-ink">“{note.quote}”</p>
              <p className="mt-1 text-right font-body text-[11px] italic tracking-[-0.12px] text-ink">
                — {note.workTitle}, {note.workAuthor}
              </p>
            </blockquote>
          ))}
        </div>
      )}
    </div>
  );
}
