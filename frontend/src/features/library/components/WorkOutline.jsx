import React from 'react';

const STATE_LABEL = {
  before: 'Antes do ponto em que você está',
  current: 'Onde você está',
  after: 'Depois do ponto em que você está',
};

/** "pág. 42" for a PDF, whose entries open at a page; the rest are told by how far through the text they start. */
function whereText(chapter) {
  const locator = chapter.locator;
  if (locator?.type === 'pdf' && Number.isInteger(locator.page)) return `pág. ${locator.page + 1}`;
  return chapter.locator ? `${Math.round(chapter.percent)}%` : '';
}

/**
 * The table of contents of the work (DEC-150): the parts and chapters of the file the person is reading, each one opening the reader
 * there, with where they are marked. Only the story is listed (a cover, a preface or an appendix is not a chapter) unless the file
 * has nothing else. "Before" and "after" are said of the place the person is at: going past a chapter is not the same as reading it.
 */
export function WorkOutline({ outline, onOpen }) {
  const currentRef = React.useRef(null);
  const all = outline?.chapters ?? [];
  const body = all.map((chapter, index) => ({ ...chapter, index })).filter((chapter) => chapter.part === 'body');
  const shown = body.length > 0 ? body : all.map((chapter, index) => ({ ...chapter, index }));

  React.useEffect(() => {
    currentRef.current?.scrollIntoView?.({ block: 'nearest' });
  }, [outline?.current, shown.length]);

  if (shown.length === 0) return null;
  const current = outline.current;
  const chapters = shown.filter((entry) => !entry.hasChildren);
  const parts = shown.filter((entry) => entry.hasChildren && entry.depth === 0);
  const count = [
    `${chapters.length} ${chapters.length === 1 ? 'capítulo' : 'capítulos'}`,
    parts.length > 0 ? `${parts.length} ${parts.length === 1 ? 'parte' : 'partes'}` : null,
  ].filter(Boolean).join(' · ');
  const stateOf = (index) => (current == null ? null : index === current ? 'current' : index < current ? 'before' : 'after');

  return (
    <section aria-label="Sumário da obra" className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <h2 className="font-display text-2xl text-ink">Sumário da obra</h2>
        <span className="font-mono text-[11px] uppercase tracking-widest text-ink-faint">{count}</span>
      </div>
      {/* relative: the hidden texts of the rows (sr-only is absolute) belong to the list, and are not left where the rows would be under the page */}
      <ol className="relative flex max-h-96 flex-col overflow-y-auto">
        {shown.map((entry) => {
          const state = entry.hasChildren ? null : stateOf(entry.index);
          const where = whereText(entry);
          const open = () => entry.locator && onOpen(entry.locator, entry.title);
          return (
            <li key={entry.index} style={{ paddingLeft: `${Math.min(entry.depth, 3) * 16}px` }}>
              <button
                ref={state === 'current' ? currentRef : undefined}
                type="button"
                onClick={open}
                disabled={!entry.locator}
                aria-current={state === 'current' ? 'location' : undefined}
                title={state ? STATE_LABEL[state] : undefined}
                className={`flex min-h-10 w-full items-center gap-3 rounded-md px-2 py-1.5 text-left text-sm hover:bg-surface-alt disabled:cursor-default disabled:hover:bg-transparent ${
                  entry.hasChildren ? 'font-mono text-[11px] font-semibold uppercase tracking-widest text-ink-faint' : 'text-ink'
                } ${state === 'current' ? 'bg-brand/10 font-semibold text-brand' : ''}`}
              >
                {!entry.hasChildren && (
                  <span aria-hidden="true" className={`w-4 shrink-0 text-center text-xs ${state === 'before' ? 'text-success' : state === 'current' ? 'text-brand' : 'text-ink-faint'}`}>
                    {state === 'before' ? '✓' : state === 'current' ? '▶' : '○'}
                  </span>
                )}
                {state && <span className="sr-only">{STATE_LABEL[state]}: </span>}
                <span className="min-w-0 flex-1 truncate">{entry.title}</span>
                {state === 'current' && <span className="shrink-0 font-mono text-[10px] uppercase tracking-wide">atual</span>}
                {where && <span className="shrink-0 font-mono text-[11px] text-ink-faint">{where}</span>}
              </button>
            </li>
          );
        })}
      </ol>
      {current != null && <p className="text-xs text-ink-faint">✓ marca o que fica antes do ponto em que você está, não o que foi lido.</p>}
    </section>
  );
}
