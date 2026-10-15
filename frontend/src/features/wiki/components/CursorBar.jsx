import React from 'react';
import { chapterName, keptBack, plural, pointOf, pointValue } from '../wiki';

const OWN = 'own';

/** Whether a point is past another: a later book, or a later chapter of the same book. */
const beyond = (a, b) => a.book > b.book || (a.book === b.book && a.chapter > b.chapter);

/**
 * The cursor of the wiki (DEC-179, DEC-183): up to where it shows, and where that comes from (the person's reading, or a point they
 * chose); a choice of another point, before or past their own, which changes nothing in their progress; the side books; and how much
 * is kept back. `cursor` is what the person chose ({ manual, book, chapter, side }); `wiki` is what the server answered.
 */
export function CursorBar({ wiki, cursor, onChange }) {
  const shown = wiki.cursor;
  const own = wiki.own;
  const main = wiki.books.filter((b) => !b.side);
  const value = cursor?.manual ? pointValue(cursor.book, cursor.chapter) : OWN;
  const back = keptBack(wiki.books, shown);
  const past = shown.source === 'manual' && own && beyond(shown, own.book < 0 ? { book: -1, chapter: 0 } : own);
  const multiple = main.length > 1;

  const choose = (event) => {
    const point = pointOf(event.target.value);
    onChange({ ...cursor, manual: !!point, book: point?.book, chapter: point?.chapter });
  };

  return (
    <section aria-label="Até onde a wiki mostra" className="rounded-xl border border-border-hairline bg-white p-4 shadow-sm sm:p-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="library-eyebrow">Sem spoiler</span>
            <span className="rounded bg-surface-alt px-2 py-0.5 font-mono text-[11px] text-ink-soft">
              {shown.source === 'manual' ? 'ponto escolhido por você' : shown.source === 'none' ? 'você ainda não começou' : 'do seu progresso no leitor'}
            </span>
          </div>
          <p className="mt-1 text-sm text-ink-soft">
            {shown.book < 0 ? (
              'Você ainda não começou a ler: a wiki mostra o que vier até onde você chegar.'
            ) : (
              <>
                Mostrando até <strong className="font-semibold text-ink">{shown.label || 'o começo'}</strong>
              </>
            )}
          </p>
        </div>

        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
          <label className="flex min-w-0 items-center gap-2 text-sm text-ink-soft">
            <span className="shrink-0">Mostrar até</span>
            <select
              value={value}
              onChange={choose}
              className="min-h-10 min-w-0 max-w-full flex-1 rounded-lg border border-border-hairline bg-white px-3 text-sm text-ink sm:max-w-xs"
            >
              <option value={OWN}>{own?.book >= 0 ? `Onde estou (${own.label})` : 'Onde estou na leitura'}</option>
              {main.map((book) => {
                const options = book.chapters.map((c) => (
                  <option key={c.chapter} value={pointValue(book.position, c.chapter)}>
                    {multiple ? `${book.title} · ` : ''}{chapterName(c)}{c.read ? '' : ' (não lido)'}
                  </option>
                ));
                return multiple ? <optgroup key={book.position} label={book.title}>{options}</optgroup> : options;
              })}
            </select>
          </label>
          {wiki.hasSide && (
            <label className="flex min-h-10 items-center gap-2 text-sm text-ink-soft">
              <input type="checkbox" checked={!!cursor?.side} onChange={(e) => onChange({ ...cursor, side: e.target.checked })} className="h-4 w-4 accent-brand" />
              Incluir contos paralelos
            </label>
          )}
        </div>
      </div>

      {(back.chapters > 0 || back.books > 0) && (
        <p className="mt-3 border-t border-border-hairline pt-3 text-[13px] text-ink-faint">
          {[back.chapters > 0 && `${plural(back.chapters, 'capítulo', 'capítulos')} deste livro`, back.books > 0 && plural(back.books, 'livro posterior', 'livros posteriores')]
            .filter(Boolean)
            .join(' e ')}{' '}
          {back.chapters + back.books === 1 ? 'fica oculto' : 'ficam ocultos'} para não contar o que vem depois.
        </p>
      )}
      {past && (
        <p role="note" className="mt-3 rounded-lg bg-warning-soft px-3 py-2 text-[13px] text-warning">
          Você escolheu ir além do que leu: o que aparece aqui pode contar o que vem depois. Seu progresso de leitura não muda.
        </p>
      )}
    </section>
  );
}
