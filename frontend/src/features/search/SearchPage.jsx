import { AuthorLinks } from '../people/components/AuthorLinks';
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { useWorks } from '../library/api/useWorks';
import { useWork } from '../reader/api/useWork';
import { useGlobalStore } from '../../store/useGlobalStore';
import { sheetNote } from '../../lib/ocr';
import { LoadError } from '../../components/ui/LoadError';
import { Notice } from '../../components/ui/Notice';
import { coverageNote } from '../../lib/processing';

const PAGE_SIZE = 10;

export function HighlightedSnippet({ text = '', matches = [] }) {
  const chars = Array.from(text);
  const spans = [];
  let at = 0;
  for (const [rawStart, rawEnd] of [...matches].sort((a, b) => a[0] - b[0])) {
    const start = Math.max(at, Math.min(chars.length, rawStart));
    const end = Math.max(start, Math.min(chars.length, rawEnd));
    if (start > at) spans.push(chars.slice(at, start).join(''));
    if (end > start) spans.push(<mark key={`${start}-${end}`} className="rounded bg-warning-soft px-0.5 text-ink">{chars.slice(start, end).join('')}</mark>);
    at = end;
  }
  if (at < chars.length) spans.push(chars.slice(at).join(''));
  return <>{spans}</>;
}

function Pager({ page, hasMore, onPageChange }) {
  if (page === 0 && !hasMore) return null;
  return <div className="flex items-center gap-3 pt-3 text-sm text-ink-soft">
    <button type="button" disabled={page === 0} onClick={() => onPageChange(page - 1)} className="disabled:opacity-40 hover:text-brand">Anterior</button>
    <span>Página {page + 1}</span>
    <button type="button" disabled={!hasMore} onClick={() => onPageChange(page + 1)} className="disabled:opacity-40 hover:text-brand">Próxima</button>
  </div>;
}

// Where the text of a passage comes from: recognised from the pictures (OCR), what a comic or an audio file says of itself
// (its ComicInfo, its chapters and its description, which belong to the whole file and not to a page), or the text of the file.
function sourceLabel(hit) {
  if (hit.origin === 'ocr') return 'Texto reconhecido por OCR';
  if (hit.locator?.type === 'audio' || hit.locator?.item === 'ComicInfo.xml') return 'Metadados do arquivo';
  return 'Texto do arquivo';
}

function FileIndexState({ file }) {
  if (file.textStatus === 'ready' && file.textSegments > 0) return null;
  let message = 'Texto ainda não indexado';
  if (file.textStatus === 'failed') message = 'Falha ao ler o texto';
  if (file.textStatus === 'empty') {
    // A scan that OCR has begun on says how far it is; one it has not touched says it needs it.
    const begun = file.ocr && (file.ocr.state || file.ocr.read || file.ocr.failed);
    message = file.needsOcr ? (begun ? sheetNote(file).text : 'Sem texto pesquisável; OCR necessário') : 'Sem texto pesquisável';
  }
  if (file.textStatus === 'ready') message = 'Nenhum texto pesquisável';
  return <li>{file.format?.toUpperCase() || 'Arquivo'}: {message}</li>;
}

export function SearchPage({ query }) {
  const [settled, setSettled] = useState(query.trim());
  const [scope, setScope] = useState(null);
  const [worksPage, setWorksPage] = useState(0);
  const [passagesPage, setPassagesPage] = useState(0);
  const [notesPage, setNotesPage] = useState(0);
  const openWork = useGlobalStore((state) => state.openWork);
  const openBook = useGlobalStore((state) => state.openBook);

  useEffect(() => {
    const timer = setTimeout(() => setSettled(query.trim().slice(0, 200)), 250);
    return () => clearTimeout(timer);
  }, [query]);
  useEffect(() => { setWorksPage(0); setPassagesPage(0); setNotesPage(0); }, [settled, scope]);

  const active = settled === query.trim().slice(0, 200) && !!settled;
  const works = useWorks({ search: settled, page: worksPage + 1, limit: PAGE_SIZE });
  const selected = useWork(scope?.id);
  const passages = useQuery({
    queryKey: ['search', 'passages', settled, scope?.id, passagesPage],
    enabled: active,
    queryFn: async () => (await api.get('/search', { params: { q: settled, workId: scope?.id, limit: PAGE_SIZE, offset: passagesPage * PAGE_SIZE } })).data,
  });
  const notes = useQuery({
    queryKey: ['search', 'notes', settled, scope?.id, notesPage],
    enabled: active,
    queryFn: async () => (await api.get('/notes', { params: { q: settled, workId: scope?.id, limit: PAGE_SIZE, offset: notesPage * PAGE_SIZE } })).data,
  });
  const workItems = scope ? (selected.data ? [selected.data] : []) : (works.data?.data ?? []);

  const chooseScope = (work) => setScope({ id: work.id, title: work.title ?? work.workTitle });
  return <div className="mx-auto flex w-full max-w-5xl flex-col gap-7 px-4 py-8 sm:px-6 lg:px-10">
    <div>
      <h1 className="font-display text-2xl text-ink">Resultados da busca</h1>
      <p className="mt-1 text-sm text-ink-soft">{scope ? `Em “${scope.title}”` : `Em todo o acervo`} · {query.trim()}</p>
      {scope && <button type="button" onClick={() => setScope(null)} className="mt-2 text-sm text-brand hover:underline">Buscar em todo o acervo</button>}
      {query.trim().length > 200 && <p role="alert" className="mt-2 text-sm text-warning">A busca usa os primeiros 200 caracteres.</p>}
    </div>

    <section aria-label="Obras" className="rounded-lg bg-white p-5 shadow-sm">
      <h2 className="font-display text-xl text-ink">Obras</h2>
      {!active || (scope ? selected.isLoading : works.isLoading) ? <p className="mt-3 text-sm text-ink-soft">Buscando obras…</p>
        : (scope ? selected.isError : works.isError) ? <LoadError className="mt-3" error={(scope ? selected : works).error} onRetry={() => (scope ? selected : works).refetch()} retrying={(scope ? selected : works).isRefetching}>Não foi possível buscar obras.</LoadError>
          : workItems.length === 0 ? <p className="mt-3 text-sm text-ink-soft">Nenhuma obra encontrada.</p>
            : <ul className="mt-3 space-y-3">{workItems.map((work) => <li key={work.id} className="rounded border border-surface-alt p-3">
              <button type="button" onClick={() => openWork(work.id)} className="font-semibold text-ink hover:text-brand">{work.title}</button>
              <p className="text-sm text-ink-soft"><AuthorLinks authors={work.authors} fallback={work.author} /></p>
              {!scope && <button type="button" onClick={() => chooseScope(work)} className="mt-1 text-xs text-brand hover:underline">Buscar só nesta obra</button>}
            </li>)}</ul>}
      {!scope && active && <Pager page={worksPage} hasMore={worksPage + 1 < (works.data?.totalPages ?? 0)} onPageChange={setWorksPage} />}
      {scope && selected.data && <div className="mt-3 text-xs text-ink-soft">
        <p>Estado do texto dos arquivos:</p>
        <ul className="mt-1 list-inside list-disc">{selected.data.editions?.flatMap((edition) => edition.files ?? []).map((file) => <FileIndexState key={file.id} file={file} />)}</ul>
      </div>}
    </section>

    <section aria-label="Passagens" className="rounded-lg bg-white p-5 shadow-sm">
      <h2 className="font-display text-xl text-ink">Passagens</h2>
      {active && !passages.isError && coverageNote(passages.data?.coverage) && <Notice tone="info" className="mt-2">{coverageNote(passages.data.coverage)}</Notice>}
      {!!passages.data?.data?.length && passages.data.mode === 'stem' && <p className="mt-1 text-xs text-ink-soft">Inclui outras formas das palavras (“correr” acha “corrida”). Para a palavra exata, use aspas.</p>}
      {!!passages.data?.data?.length && passages.data.mode === 'exact' && <p className="mt-1 text-xs text-ink-soft">Busca exata: só as palavras como estão escritas.</p>}
      {!active || passages.isLoading ? <p className="mt-3 text-sm text-ink-soft">Buscando passagens…</p>
        : passages.isError ? <LoadError className="mt-3" error={passages.error} onRetry={() => passages.refetch()} retrying={passages.isRefetching}>Não foi possível buscar passagens.</LoadError>
          : !passages.data?.data?.length ? <p className="mt-3 text-sm text-ink-soft">Nenhuma passagem encontrada.</p>
            : <ul className="mt-3 space-y-3">{passages.data.data.map((hit) => <li key={hit.segmentId} className="rounded border border-surface-alt p-3">
              <p className="text-sm font-semibold text-ink">{hit.workTitle} <span className="font-normal text-ink-soft">· <AuthorLinks authors={hit.workAuthors} fallback={hit.workAuthor} /></span></p>
              <p className="text-xs text-ink-soft">{hit.format?.toUpperCase()}{hit.language ? ` · ${hit.language.toUpperCase()}` : ''}{hit.section ? ` · ${hit.section}` : ''} · {sourceLabel(hit)}</p>
              <p className="mt-2 whitespace-pre-wrap text-sm text-ink"><HighlightedSnippet text={hit.snippet} matches={hit.matches} /></p>
              <div className="mt-2 flex gap-4 text-xs text-brand">
                <button type="button" onClick={() => openBook(hit.workId, hit.fileId, { locator: hit.locator, context: { kind: 'search', quote: hit.snippet || '' } })} className="hover:underline">Abrir neste ponto</button>
                {!scope && <button type="button" onClick={() => chooseScope(hit)} className="hover:underline">Buscar só nesta obra</button>}
              </div>
            </li>)}</ul>}
      {active && <Pager page={passagesPage} hasMore={!!passages.data?.hasMore} onPageChange={setPassagesPage} />}
    </section>

    <section aria-label="Anotações" className="rounded-lg bg-white p-5 shadow-sm">
      <h2 className="font-display text-xl text-ink">Suas anotações</h2>
      {!active || notes.isLoading ? <p className="mt-3 text-sm text-ink-soft">Buscando anotações…</p>
        : notes.isError ? <LoadError className="mt-3" error={notes.error} onRetry={() => notes.refetch()} retrying={notes.isRefetching}>Não foi possível buscar anotações.</LoadError>
          : !notes.data?.data?.length ? <p className="mt-3 text-sm text-ink-soft">Nenhuma anotação encontrada.</p>
            : <ul className="mt-3 space-y-3">{notes.data.data.map((note) => <li key={note.id} className="rounded border border-surface-alt p-3">
              <p className="text-sm font-semibold text-ink">{note.workTitle} <span className="font-normal text-ink-soft">· {note.workAuthor}</span></p>
              {note.quote && <p className="mt-1 text-sm text-ink-soft">“{note.quote}”</p>}
              {note.body && <p className="mt-1 whitespace-pre-wrap text-sm text-ink">{note.body}</p>}
              {note.tags?.length > 0 && <p className="mt-1 text-xs text-ink-soft">{note.tags.join(' · ')}</p>}
              {note.sourceAvailable && note.fileAvailable && note.fileId && note.locator
                ? <button type="button" onClick={() => openBook(note.workId, note.fileId, { locator: note.locator, context: { kind: 'note', quote: note.quote } })} className="mt-2 text-xs text-brand hover:underline">Abrir neste ponto</button>
                : <p className="mt-2 text-xs text-ink-soft">{note.sourceAvailable ? 'Anotação sem posição para abrir' : 'Fonte indisponível'}</p>}
            </li>)}</ul>}
      {active && <Pager page={notesPage} hasMore={(notesPage + 1) * PAGE_SIZE < (notes.data?.total ?? 0)} onPageChange={setNotesPage} />}
    </section>
    <p className="text-xs text-ink-soft">A busca textual não relaciona formas como “correr” e “corrida”. PDFs escaneados só aparecem nas passagens após OCR.</p>
  </div>;
}
