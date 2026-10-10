import React from 'react';

const SHOWN = 6;
const QUOTE_FROM = 30; // a quote shorter than this is a word or two, not something to put at the top of a page
const QUOTE_TO = 400;

/** The notes of the work that have words of the book in them: a highlight or a note with the passage, not a bookmark. */
export function highlightsOf(notes) {
  return (notes ?? []).filter((note) => note.kind !== 'bookmark' && (note.quote || '').trim() !== '');
}

/** "pág. 42" for a note in a PDF; an EPUB has no pages, and its chapter says where it is. */
function pageOf(note) {
  const locator = note.locator;
  return locator?.type === 'pdf' && Number.isInteger(locator.page) ? `pág. ${locator.page + 1}` : '';
}

/** Where a note is, as one short line: the chapter and the page. */
function whereLine(note) {
  return [note.chapter, pageOf(note)].filter(Boolean).join(' · ');
}

/**
 * One of the person's highlights, chosen when the page opens, to put under the tags: "alguma que a pessoa tenha salvo durante sua leitura".
 * Another visit may bring another. Only a passage of a length that reads well alone is chosen.
 */
export function FeaturedQuote({ notes, workId, onOpen }) {
  const chosen = React.useMemo(() => {
    const candidates = highlightsOf(notes).filter((note) => {
      const length = note.quote.trim().length;
      return length >= QUOTE_FROM && length <= QUOTE_TO;
    });
    if (candidates.length === 0) return null;
    return candidates[Math.floor(Math.random() * candidates.length)];
    // chosen again only for another work or when the notes themselves change: not on every render
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workId, notes?.map((note) => note.id).join(',')]);
  if (!chosen) return null;
  const where = whereLine(chosen);
  return (
    <figure className="max-w-2xl rounded-xl border-l-4 border-brand bg-white p-4 shadow-sm" aria-label="Um destaque seu">
      <blockquote className="font-display text-lg italic leading-relaxed text-ink sm:text-xl">“{chosen.quote.trim()}”</blockquote>
      <figcaption className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-ink-soft">
        <span>{where || 'Um trecho que você destacou'}</span>
        <button type="button" onClick={() => onOpen(chosen)} className="underline decoration-dotted underline-offset-4 hover:text-ink">
          Abrir no livro
        </button>
      </figcaption>
    </figure>
  );
}

/** The person's highlights and notes in the work, each opening the reader at its place. */
export function WorkHighlights({ notes, onOpen }) {
  const [all, setAll] = React.useState(false);
  const list = highlightsOf(notes);
  if (list.length === 0) return null;
  const shown = all ? list : list.slice(0, SHOWN);
  return (
    <section aria-label="Seus destaques" className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <h2 className="font-display text-2xl text-ink">Seus destaques</h2>
        <span className="font-mono text-[11px] uppercase tracking-widest text-ink-faint">
          {list.length} {list.length === 1 ? 'anotação salva' : 'anotações salvas'}
        </span>
      </div>
      <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {shown.map((note) => {
          const where = whereLine(note);
          return (
            <li key={note.id} className="flex flex-col gap-2 rounded-lg bg-surface p-3">
              <button type="button" onClick={() => onOpen(note)} className="text-left" title="Abrir no livro">
                <span className="line-clamp-5 font-display text-base italic leading-snug text-ink">“{note.quote.trim()}”</span>
              </button>
              {note.body?.trim() && <p className="line-clamp-3 text-sm text-ink-soft">{note.body.trim()}</p>}
              <div className="mt-auto flex flex-wrap items-center justify-between gap-2 font-mono text-[11px] text-ink-faint">
                <span>{where}</span>
                {note.tags?.length > 0 && <span>{note.tags.map((tag) => `#${tag}`).join(' ')}</span>}
              </div>
            </li>
          );
        })}
      </ul>
      {list.length > SHOWN && (
        <button type="button" onClick={() => setAll(!all)} aria-expanded={all} className="self-start text-xs text-ink-soft underline decoration-dotted underline-offset-4 hover:text-ink">
          {all ? 'Mostrar menos' : `Mostrar todos (${list.length})`}
        </button>
      )}
    </section>
  );
}
