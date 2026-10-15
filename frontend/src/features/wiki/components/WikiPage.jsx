import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { isStaff, useMe } from '../../auth/api/useMe';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useWiki, useWikiGraph } from '../api/useWiki';
import { kindCounts, providerName } from '../wiki';
import { CursorBar } from './CursorBar';
import { WikiEntity } from './WikiEntity';
import { WikiGraph } from './WikiGraph';
import { ReviewBanner } from './ReviewBanner';
import { WikiIndex } from './WikiIndex';

const TAB = 'inline-flex min-h-10 items-center gap-2 whitespace-nowrap rounded-lg border px-4 text-sm';
const TAB_ON = 'border-brand bg-brand text-white';
const TAB_OFF = 'border-border-hairline bg-white text-ink-soft hover:text-ink';

const DATE = new Intl.DateTimeFormat('pt-BR', { day: 'numeric', month: 'long', year: 'numeric' });

const OWN_CURSOR = { manual: false, side: false };
const cursorKey = (workId) => `codice:wiki-cursor:${workId}`;

/**
 * The point chosen in the cursor control, kept in the session of the tab (per work) so that F5 does not lose it. Not in the address:
 * a link sent to someone else must not carry a point past what they read.
 */
function savedCursor(workId) {
  try {
    const value = JSON.parse(sessionStorage.getItem(cursorKey(workId)) ?? 'null');
    if (value && typeof value === 'object') {
      const manual = !!value.manual && Number.isInteger(value.book) && Number.isInteger(value.chapter);
      return { manual, side: !!value.side, ...(manual ? { book: value.book, chapter: value.chapter } : {}) };
    }
  } catch {
    // No storage, or something else in it: the reader's own point.
  }
  return OWN_CURSOR;
}

function saveCursor(workId, cursor) {
  try {
    if (!cursor.manual && !cursor.side) sessionStorage.removeItem(cursorKey(workId));
    else sessionStorage.setItem(cursorKey(workId), JSON.stringify(cursor));
  } catch {
    // Without storage the choice lasts until the page is reloaded.
  }
}

/** What the state of the generation means for who reads (DEC-183): only what is not simply "ready" is said. */
function StateNotice({ project, staff }) {
  const notices = [];
  if (project.state === 'generating') notices.push('A wiki está sendo gerada de novo. O que aparece aqui é a versão anterior.');
  if (project.state === 'partial') notices.push('Esta wiki está parcial: alguma etapa da geração não terminou, e podem faltar páginas ou vínculos.');
  if (project.state === 'failed') notices.push('A última geração falhou. O que aparece aqui é a versão anterior.');
  if (project.review === 'pending' && !staff) notices.push('Esta wiki foi atualizada com um livro novo e está em revisão.');
  if (notices.length === 0) return null;
  return (
    <div role="status" className="rounded-lg bg-warning-soft px-4 py-3 text-sm text-warning">
      {notices.map((text) => (
        <p key={text}>{text}</p>
      ))}
    </div>
  );
}

/** Who wrote the wiki and where (DEC-177): the real provider and model, when, and whether the text of the books left the server. */
function ProviderNote({ project }) {
  return (
    <section aria-label="Sobre esta wiki" className="rounded-xl border border-border-hairline bg-surface/60 px-5 py-4 text-[13px] text-ink-soft">
      <p>
        <span className="mr-2 rounded bg-brand px-2 py-0.5 font-mono text-[11px] text-white">Experimental</span>
        Gerada automaticamente por <strong className="text-ink">{providerName(project.provider)}</strong> com o modelo{' '}
        <strong className="text-ink">{project.model}</strong>
        {project.generatedAt ? `, em ${DATE.format(new Date(project.generatedAt))}` : ''}.
      </p>
      <p className="mt-1">
        {project.remote
          ? 'O texto dos livros saiu deste servidor para ser lido pelo provedor. '
          : 'O texto dos livros não saiu deste servidor. '}
        Cada frase se apoia em trechos do livro conferidos no texto, mas pode haver erros.
      </p>
    </section>
  );
}

/**
 * The automatic wiki of a work or of its series (RF-050, RF-051, DEC-177 to DEC-183): the index of who and what appeared up to the
 * reader's point, the page of each entity and the graph. It takes the place of the page of the work; the reader opens over it (a
 * reference opens the book at the paragraph, without moving the reading position), and closing the book comes back here.
 */
export function WikiPage() {
  const workId = useGlobalStore((state) => state.wikiWorkId);
  const slug = useGlobalStore((state) => state.wikiEntity);
  const graphOpen = useGlobalStore((state) => state.wikiGraph);
  const from = useGlobalStore((state) => state.wikiFrom);
  const openEntity = useGlobalStore((state) => state.openWikiEntity);
  const openGraph = useGlobalStore((state) => state.openWikiGraph);
  const openWork = useGlobalStore((state) => state.openWork);
  const openBook = useGlobalStore((state) => state.openBook);
  const resume = useGlobalStore((state) => state.resumeFromWiki);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const staff = isStaff(useMe().data);
  const pageRef = React.useRef(null);
  const [cursor, setCursorState] = React.useState(() => savedCursor(workId));
  const [kind, setKind] = React.useState('');
  const setCursor = (next) => {
    setCursorState(next);
    saveCursor(workId, next);
  };

  // Another work starts again from the point chosen for it in this tab (or the reader's own), with every kind.
  React.useEffect(() => {
    setCursorState(savedCursor(workId));
    setKind('');
  }, [workId]);

  const { data, isLoading, isError, error, refetch, isRefetching } = useWiki(workId, cursor);
  const wiki = data?.wiki;
  const graph = useWikiGraph(wiki?.project.id, cursor, { enabled: !!wiki && (graphOpen || !!slug) });

  // Each place of the wiki opens at its top.
  React.useEffect(() => {
    pageRef.current?.scrollIntoView?.({ block: 'start' });
  }, [workId, slug, graphOpen]);

  if (!workId) return null;

  const title = wiki?.books.find((b) => b.workId === workId)?.title ?? wiki?.books[0]?.title;
  const main = wiki?.books.filter((b) => !b.side) ?? [];
  const where = graphOpen ? 'Grafo' : slug ? 'Página' : null;
  const back = () => (graphOpen ? (slug ? openEntity(slug) : openEntity(null)) : slug ? openEntity(null) : openWork(workId));
  const chooseKind = (key) => {
    setKind(key);
    if (slug && !graphOpen) openEntity(null);
  };
  const openAt = (book, item) =>
    openBook(book.workId, book.fileId, { locator: item.locator, context: { kind: 'wiki', quote: item.quote ?? '' }, peek: true });
  const counts = kindCounts(wiki?.index);

  return (
    <div ref={pageRef} className="library-dashboard" role="region" aria-label="Wiki">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <nav aria-label="Onde você está" className="flex min-w-0 flex-wrap items-center gap-2 text-[13px] text-ink-soft">
          <button className="library-button" onClick={back}>← Voltar</button>
          <button className="library-text-link" style={{ marginLeft: 0 }} onClick={() => setView('all')}>Biblioteca</button>
          <span aria-hidden="true">›</span>
          <button className="library-text-link min-w-0 truncate" style={{ marginLeft: 0 }} onClick={() => openWork(workId)}>{title ?? 'Obra'}</button>
          <span aria-hidden="true">›</span>
          {where ? (
            <>
              <button className="library-text-link" style={{ marginLeft: 0 }} onClick={() => openEntity(null)}>Wiki</button>
              <span aria-hidden="true">›</span>
              <span aria-current="page" className="min-w-0 truncate text-ink">{where === 'Grafo' ? 'Grafo' : wiki?.index.find((e) => e.slug === slug)?.name ?? slug}</span>
            </>
          ) : (
            <span aria-current="page" className="text-ink">Wiki</span>
          )}
        </nav>
        {from && (
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-[11px] text-ink-faint">Você veio do leitor{wiki?.own?.book >= 0 && wiki.own.label ? ` · ${wiki.own.label}` : ''}</span>
            <button type="button" className="library-button" onClick={resume}>
              Retomar leitura no parágrafo →
            </button>
          </div>
        )}
      </div>

      {isLoading && <p className="animate-pulse py-6 text-sm text-ink-faint">Carregando a wiki…</p>}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir a wiki.</LoadError>}
      {data && !wiki && (
        <p className="mt-6 rounded-lg border border-dashed border-border-hairline bg-surface/50 px-4 py-8 text-center text-sm text-ink-faint">
          Esta obra ainda não tem wiki.
        </p>
      )}
      {wiki && (
        <div className="mt-4 flex flex-col gap-5">
          {!slug && !graphOpen && (
            <header className="flex flex-wrap items-end justify-between gap-3">
              <div className="min-w-0">
                <p className="library-eyebrow">Wiki · experimental</p>
                <h1 className="font-display text-4xl leading-tight text-ink sm:text-5xl">{main.length > 1 ? main.map((b) => b.title).join(' · ') : title}</h1>
              </div>
            </header>
          )}
          <StateNotice project={wiki.project} staff={staff} />
          {staff && wiki.project.review === 'pending' && <ReviewBanner projectId={wiki.project.id} onOpen={openEntity} />}
          <CursorBar wiki={wiki} cursor={cursor} onChange={setCursor} />

          <div className="flex flex-wrap items-center justify-between gap-3">
            {graphOpen || !slug ? (
              <div role="group" aria-label="Tipo de entidade" className="-mx-1 flex max-w-full gap-2 overflow-x-auto px-1 pb-1 sm:flex-wrap">
                <button
                  type="button"
                  onClick={() => chooseKind('')}
                  aria-pressed={kind === ''}
                  className="min-h-10 shrink-0 whitespace-nowrap rounded-full border border-border-hairline bg-white px-4 text-sm text-ink hover:bg-surface-alt aria-pressed:border-brand aria-pressed:bg-brand aria-pressed:text-white"
                >
                  Todas<span className="ml-2 font-mono text-[11px] opacity-70">{wiki.index.length}</span>
                </button>
                {counts.map((k) => (
                  <button
                    key={k.key}
                    type="button"
                    onClick={() => chooseKind(k.key)}
                    aria-pressed={kind === k.key}
                    className="min-h-10 shrink-0 whitespace-nowrap rounded-full border border-border-hairline bg-white px-4 text-sm text-ink hover:bg-surface-alt aria-pressed:border-brand aria-pressed:bg-brand aria-pressed:text-white"
                  >
                    {k.many}<span className="ml-2 font-mono text-[11px] opacity-70">{k.count}</span>
                  </button>
                ))}
              </div>
            ) : (
              <span />
            )}
            <div role="tablist" aria-label="Vista" className="flex gap-2">
              {[
                { key: 'index', label: 'Índice', on: !slug && !graphOpen, go: () => openEntity(null) },
                ...(slug ? [{ key: 'page', label: 'Página', on: !graphOpen, go: () => openEntity(slug) }] : []),
                { key: 'graph', label: 'Grafo', on: graphOpen, go: openGraph },
              ].map((t) => (
                <button key={t.key} type="button" role="tab" aria-selected={t.on} onClick={t.go} className={`${TAB} ${t.on ? TAB_ON : TAB_OFF}`}>
                  {t.label}
                </button>
              ))}
            </div>
          </div>

          {graphOpen ? (
            <WikiGraph query={graph} kind={kind} focus={slug} onOpen={openEntity} />
          ) : slug ? (
            <WikiEntity
              project={wiki.project}
              books={wiki.books}
              index={wiki.index}
              slug={slug}
              cursor={cursor}
              graph={graph.data}
              staff={staff}
              onOpen={openEntity}
              onExpandGraph={openGraph}
              onOpenAt={openAt}
            />
          ) : (
            <WikiIndex index={wiki.index} kind={kind} onOpen={openEntity} staff={staff} projectId={wiki.project.id} books={wiki.books} shown={wiki.cursor} />
          )}

          <ProviderNote project={wiki.project} />
        </div>
      )}
    </div>
  );
}
