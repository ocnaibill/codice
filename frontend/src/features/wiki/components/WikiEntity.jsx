import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { useHideSentence, useWikiEntity } from '../api/useWiki';
import { GROUPS, kindName, plural, pointLabel } from '../wiki';
import { IdentityPanel } from './IdentityPanel';
import { MiniGraph } from './MiniGraph';

const CARD = 'rounded-xl border border-border-hairline bg-white p-5 shadow-sm lg:p-6';
const H2 = 'font-display text-2xl leading-tight text-ink';

/** The chapter part of a label ("Livro, Parte · Capítulo 3" → "Capítulo 3"). */
const chapterOf = (label) => String(label ?? '').split(' · ').pop();

const refId = (n) => `wiki-ref-${n}`;

// The reference whose paragraph was opened in the reader: coming back from the book, the page opens at it, not at its top.
let openedReference = null;

/** Goes to a reference below and gives it the focus (the page is under a hash route, so not a link to an anchor). */
function goToReference(n) {
  const target = document.getElementById(refId(n));
  target?.scrollIntoView?.({ block: 'center', behavior: 'smooth' });
  target?.focus?.({ preventScroll: true });
}

// What the staff can do to a sentence (RF-052): `mark(sentence, hidden)`; null for a reader, who gets no button.
const Correction = React.createContext(null);

function Sentences({ sentences }) {
  const correct = React.useContext(Correction);
  return sentences.map((s, i) => (
    <React.Fragment key={s.id}>
      {i > 0 && ' '}
      {s.hidden ? <del className="text-ink-faint" title="Marcada como errada: nenhum leitor a vê">{s.text}</del> : s.text}
      {s.hidden && correct && (
        <button type="button" onClick={() => correct.mark(s, false)} className="ml-1 text-[12px] text-brand hover:underline">
          Mostrar de novo
        </button>
      )}
      {!s.hidden && correct && (
        <button
          type="button"
          onClick={() => correct.mark(s, true)}
          title="Isto está errado? Marque a frase e ela deixa de aparecer para todos."
          aria-label="Marcar esta frase como errada"
          className="ml-0.5 align-super text-[11px] text-ink-faint hover:text-danger"
        >
          ⚑
        </button>
      )}
      {s.refs.map((n) => (
        <sup key={n} className="ml-0.5 font-mono text-[10px]">
          <button type="button" onClick={() => goToReference(n)} className="text-brand hover:underline" aria-label={`Referência ${n}`}>
            [{n}]
          </button>
        </sup>
      ))}
    </React.Fragment>
  ));
}

function Paragraphs({ paragraphs, className = '' }) {
  return paragraphs.map((p, i) => (
    <p key={i} className={`text-[15px] leading-7 text-ink ${className}`}>
      <Sentences sentences={p} />
    </p>
  ));
}

function Header({ page, cursor, books }) {
  return (
    <div className={CARD}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="library-eyebrow">{kindName(page.kind)}</span>
        {!page.page && <span className="rounded bg-surface-alt px-2 py-0.5 font-mono text-[11px] text-ink-soft">sem página completa</span>}
      </div>
      <h1 tabIndex={-1} data-wiki-heading className="mt-2 font-display text-4xl leading-tight text-ink outline-none sm:text-5xl">{page.name}</h1>
      {(page.books.length > 0 || page.hidden?.laterBooks > 0) && (
        <ul aria-label="Onde aparece" className="mt-4 flex flex-wrap gap-2">
          {page.books.map((b) => (
            <li key={b.book} className="rounded bg-surface-alt px-2.5 py-1 font-mono text-[11px] text-ink-soft">
              {b.title}{b.side ? ' (conto paralelo)' : ''} · {plural(b.chapters, 'capítulo', 'capítulos')}
            </li>
          ))}
          {page.hidden?.laterBooks > 0 && (
            <li className="rounded border border-dashed border-border-hairline px-2.5 py-1 font-mono text-[11px] text-ink-faint">
              {plural(page.hidden.laterBooks, 'livro depois', 'livros depois')}, oculto{page.hidden.laterBooks === 1 ? '' : 's'}
            </li>
          )}
        </ul>
      )}
      {page.situation && (
        <div className="mt-5 border-l-4 border-brand bg-[var(--library-paper)] px-4 py-3">
          <p className="library-eyebrow">Situação em {pointLabel(page.situation.label, books) || cursor.label}</p>
          <p className="mt-1 font-display text-lg italic leading-snug text-ink">“{page.situation.text}”</p>
        </div>
      )}
    </div>
  );
}

function Who({ who, books }) {
  return (
    <section aria-labelledby="wiki-who" className={CARD}>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 id="wiki-who" className={H2}>Quem é</h2>
        <span className="font-mono text-[11px] text-ink-faint">atualizado ao fim de {pointLabel(who.label, books)}</span>
      </div>
      <div className="mt-3 flex flex-col gap-3">
        <Paragraphs paragraphs={who.paragraphs} />
      </div>
    </section>
  );
}

function Biography({ biography, inProgress, hidden, cursor }) {
  const parts = biography.reduce((n, b) => n + b.parts.length, 0);
  if (parts === 0 && !inProgress && !hidden) return null;
  return (
    <section aria-labelledby="wiki-bio" className={CARD}>
      <h2 id="wiki-bio" className={H2}>Biografia</h2>
      <div className="mt-4 flex flex-col gap-4">
        {biography.map((book) => (
          <details key={book.book} open className="group rounded-lg border border-border-hairline">
            <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-3 rounded-lg bg-surface/60 px-4 py-2 text-sm font-semibold text-ink">
              <span>{book.title}{book.side ? ' · conto paralelo' : ''}</span>
              <span aria-hidden="true" className="text-ink-faint transition-transform group-open:rotate-180">⌄</span>
            </summary>
            <div className="flex flex-col gap-5 px-4 py-4">
              {book.parts.map((part) => (
                <div key={`${part.part}-${part.chapterFrom}`}>
                  <h3 className="flex flex-wrap items-baseline gap-x-2 font-display text-lg text-ink">
                    {part.part || 'Capítulos'}
                    <span className="font-mono text-[11px] font-normal text-ink-faint">
                      {part.chapterFrom === part.chapterTo ? `cap. ${part.chapterFrom}` : `caps. ${part.chapterFrom}–${part.chapterTo}`}
                    </span>
                  </h3>
                  <div className="mt-2 flex flex-col gap-3">
                    <Paragraphs paragraphs={part.paragraphs} />
                  </div>
                </div>
              ))}
            </div>
          </details>
        ))}

        {inProgress && (
          <div className="rounded-lg border border-brand/30 bg-[var(--library-paper)] p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm font-semibold text-ink">
                {inProgress.part ? `${inProgress.part}: ` : ''}em andamento
              </p>
              <span className="font-mono text-[11px] text-brand">você está em {chapterOf(cursor.label) || `capítulo ${cursor.chapter}`}</span>
            </div>
            <p className="mt-1 text-[13px] text-ink-faint">A biografia desta parte aparece quando você terminar de lê-la. Por enquanto, capítulo a capítulo:</p>
            {inProgress.states.length > 0 ? (
              <ol className="mt-3 flex flex-col gap-3 border-l border-border-hairline pl-4">
                {inProgress.states.map((s) => {
                  const here = s.book === cursor.book && s.chapter === cursor.chapter;
                  return (
                    <li key={`${s.book}-${s.chapter}`} aria-current={here ? 'step' : undefined} className="relative">
                      <span aria-hidden="true" className={`absolute -left-[21px] top-1.5 h-2.5 w-2.5 rounded-full ${here ? 'bg-brand' : 'bg-border-hairline'}`} />
                      <p className="font-mono text-[11px] text-ink-faint">
                        {chapterOf(s.label)}
                        {here && <span className="ml-2 text-brand">ponto atual</span>}
                      </p>
                      <p className="text-[15px] leading-6 text-ink">{s.text}</p>
                    </li>
                  );
                })}
              </ol>
            ) : (
              <p className="mt-3 text-sm text-ink-faint">Nada registrado nesta parte até aqui.</p>
            )}
          </div>
        )}

        {hidden && (hidden.chapterFrom > 0 || hidden.laterBooks > 0) && (
          <div className="rounded-lg border border-dashed border-border-hairline px-4 py-5 text-center">
            <p className="text-sm font-semibold text-ink-soft">
              {hidden.chapterFrom > 0
                ? `${hidden.chapterFrom === hidden.chapterTo ? 'O próximo capítulo está oculto' : `Os próximos ${hidden.chapterTo - hidden.chapterFrom + 1} capítulos estão ocultos`}${
                    hidden.laterBooks > 0 ? `, e ${plural(hidden.laterBooks, 'livro depois', 'livros depois')}` : ''
                  }`
                : `${plural(hidden.laterBooks, 'livro depois oculto', 'livros depois ocultos')}`}
            </p>
            <p className="mt-1 text-[13px] text-ink-faint">O que vem depois fica guardado até você ler. Para ver agora, escolha ir além no controle do topo.</p>
          </div>
        )}
      </div>
    </section>
  );
}

function Personality({ sentences }) {
  if (sentences.length === 0) return null;
  return (
    <section aria-labelledby="wiki-personality" className={CARD}>
      <h2 id="wiki-personality" className={H2}>Personalidade</h2>
      <p className="mt-3 text-[15px] leading-7 text-ink">
        <Sentences sentences={sentences} />
      </p>
    </section>
  );
}

function Timeline({ timeline, cursor, books }) {
  if (timeline.length === 0) return null;
  return (
    <details className={`${CARD} group`}>
      <summary className="flex min-h-10 cursor-pointer list-none flex-wrap items-center justify-between gap-2">
        <span className="flex flex-wrap items-baseline gap-x-3">
          <span className={H2}>Linha do tempo</span>
          <span className="font-mono text-[11px] text-ink-faint">{plural(timeline.length, 'registro', 'registros')} até {chapterOf(cursor.label)}</span>
        </span>
        <span className="text-sm text-brand">
          <span className="group-open:hidden">Mostrar</span>
          <span className="hidden group-open:inline">Recolher</span>
        </span>
      </summary>
      <ol className="mt-4 flex flex-col gap-3">
        {timeline.map((s) => (
          <li key={`${s.book}-${s.chapter}`} className="grid gap-1 sm:grid-cols-[180px_minmax(0,1fr)] sm:gap-4">
            <span className="font-mono text-[11px] text-ink-faint">{pointLabel(s.label, books)}</span>
            <span className="text-[15px] leading-6 text-ink">{s.text}</span>
          </li>
        ))}
      </ol>
    </details>
  );
}

function References({ slug, references, books, onOpenAt }) {
  if (references.length === 0) return null;
  return (
    <section aria-labelledby="wiki-refs" className={CARD}>
      <h2 id="wiki-refs" className={H2}>Referências</h2>
      <p className="mt-1 text-[13px] text-ink-faint">Cada frase acima se apoia nestes trechos do livro, conferidos no texto.</p>
      <ol className="mt-4 flex flex-col gap-4">
        {references.map((ref) => {
          const book = books.find((b) => b.position === ref.book);
          return (
            <li key={ref.n} id={refId(ref.n)} tabIndex={-1} className="rounded-lg border border-border-hairline p-4 outline-none focus:border-brand">
              <p className="font-mono text-[12px] text-ink-soft">
                <span className="text-brand">[{ref.n}]</span> {pointLabel(ref.label, books)}
              </p>
              <ul className="mt-2 flex flex-col gap-3">
                {ref.items.map((item, i) => (
                  <li key={i}>
                    {item.quote && (
                      <blockquote className="border-l-2 border-brand/40 pl-3 font-display text-[15px] italic leading-6 text-ink">“{item.quote}”</blockquote>
                    )}
                    <p className="mt-1 text-[13px] text-ink-faint">{item.fact}</p>
                    {item.locator && book?.fileId && (
                      <button
                        type="button"
                        onClick={() => {
                          openedReference = { slug, n: ref.n };
                          onOpenAt(book, item);
                        }}
                        className="mt-1 inline-flex min-h-9 items-center text-[13px] font-semibold text-brand hover:underline"
                      >
                        Abrir no leitor neste parágrafo →
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

function Ficha({ page, onOpen }) {
  const groups = page.relations.filter((g) => g.items.length > 0);
  return (
    <section aria-labelledby="wiki-ficha" className={CARD}>
      <h2 id="wiki-ficha" className="font-display text-xl text-ink">Ficha</h2>
      {page.aliases.length > 0 && (
        <div className="mt-4">
          <p className="library-eyebrow mb-2">Também chamado</p>
          <ul className="flex flex-wrap gap-2">
            {page.aliases.map((a) => (
              <li key={a} className="rounded bg-surface-alt px-2.5 py-1 text-[13px] text-ink-soft">{a}</li>
            ))}
          </ul>
        </div>
      )}
      {groups.length === 0 && page.aliases.length === 0 && <p className="mt-3 text-sm text-ink-faint">Sem vínculos registrados até aqui.</p>}
      {groups.map((g) => (
        <div key={g.group} className="mt-4">
          <p className="library-eyebrow mb-2">{GROUPS[g.group] ?? g.group}</p>
          <ul className="flex flex-col gap-1.5">
            {g.items.map((item, i) => (
              <li key={`${item.slug ?? item.name}-${i}`} className="text-sm leading-snug">
                {item.slug ? (
                  <button
                    type="button"
                    onClick={() => onOpen(item.slug)}
                    title={item.page ? undefined : 'Sem página completa: abre a ficha e a linha do tempo'}
                    className={`text-left text-brand hover:underline ${item.page ? 'font-semibold' : ''}`}
                  >
                    {item.name}
                  </button>
                ) : (
                  <span className="font-semibold text-ink">{item.name}</span>
                )}
                {item.role && <span className="text-ink-faint"> · {item.role}</span>}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </section>
  );
}

/**
 * The page of an entity (DEC-178, DEC-183): header with the situation at this point, "Quem é", the biography of the parts read, the
 * part in progress chapter by chapter and what is kept back, personality, timeline and references; at the side the ficha, the
 * minigraph and, for staff, the identity suggestions. On a phone the ficha comes after "Quem é", as the box of a wiki comes after its lead.
 */
export function WikiEntity({ project, books, index, slug, cursor, graph, staff = false, onOpen, onExpandGraph, onOpenAt }) {
  const { data, isLoading, isError, error, refetch, isRefetching } = useWikiEntity(project.id, slug, cursor);
  const { mutate: setHidden } = useHideSentence(project.id);
  const correction = React.useMemo(() => (staff ? { mark: (sentence, hidden) => setHidden({ id: sentence.id, hidden }) } : null), [staff, setHidden]);

  // A page that opens gives the focus to its name, for who goes by the keyboard or a screen reader.
  const loaded = !!data;
  React.useEffect(() => {
    if (!loaded) return;
    const back = openedReference?.slug === slug ? openedReference.n : null;
    openedReference = null;
    // After the page has gone to its top (the effect of the page around this one runs after this one).
    const timer = setTimeout(() => {
      const target = back ? document.getElementById(refId(back)) : null;
      if (target) {
        target.scrollIntoView?.({ block: 'center' });
        target.focus?.({ preventScroll: true });
      } else {
        document.querySelector('[data-wiki-heading]')?.focus?.({ preventScroll: true });
      }
    }, 0);
    return () => clearTimeout(timer);
  }, [slug, loaded]);

  if (isLoading) return <p className="animate-pulse py-6 text-sm text-ink-faint">Carregando a página…</p>;
  if (isError) {
    if (error?.response?.status === 404) {
      return (
        <p className="rounded-lg border border-dashed border-border-hairline bg-surface/50 px-4 py-8 text-center text-sm text-ink-faint">
          Esta página não existe, ou fica além do ponto em que você está.
        </p>
      );
    }
    return <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta página da wiki.</LoadError>;
  }
  const page = data.page;
  const shown = data.cursor;

  return (
    <Correction.Provider value={correction}>
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:grid-rows-[auto_auto_1fr]">
      <div className="lg:col-span-2">
        <Header page={page} cursor={shown} books={books} />
      </div>
      {page.who && (
        <div className="min-w-0 lg:col-start-1 lg:row-start-2">
          <Who who={page.who} books={books} />
        </div>
      )}
      <aside aria-label="Ficha e vínculos" className="flex flex-col gap-6 lg:col-start-2 lg:row-span-2 lg:row-start-2 lg:self-start">
        <Ficha page={page} onOpen={onOpen} />
        <MiniGraph graph={graph} slug={page.slug} onOpen={onOpen} onExpand={onExpandGraph} />
        {staff && <IdentityPanel projectId={project.id} page={page} index={index} onOpen={onOpen} />}
      </aside>
      <article className={`flex min-w-0 flex-col gap-6 lg:col-start-1 ${page.who ? 'lg:row-start-3' : 'lg:row-start-2'}`}>
        <Biography biography={page.biography} inProgress={page.inProgress} hidden={page.hidden} cursor={shown} />
        <Personality sentences={page.personality} />
        <Timeline timeline={page.timeline} cursor={shown} books={books} />
        <References slug={page.slug} references={page.references} books={books} onOpenAt={onOpenAt} />
        {!page.page && !page.who && page.timeline.length === 0 && (
          <p className="text-sm text-ink-faint">Esta entidade aparece no livro, mas não tem página completa: veja os vínculos na ficha.</p>
        )}
      </article>
    </div>
    </Correction.Provider>
  );
}
