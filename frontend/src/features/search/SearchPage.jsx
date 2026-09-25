import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { useWorks } from '../library/api/useWorks';
import { useWork } from '../reader/api/useWork';
import { useGlobalStore } from '../../store/useGlobalStore';

const PAGE_SIZE = 10;

export function HighlightedSnippet({ text = '', matches = [] }) {
  const chars = Array.from(text);
  const spans = [];
  let at = 0;
  for (const [rawStart, rawEnd] of [...matches].sort((a, b) => a[0] - b[0])) {
    const start = Math.max(at, Math.min(chars.length, rawStart));
    const end = Math.max(start, Math.min(chars.length, rawEnd));
    if (start > at) spans.push(chars.slice(at, start).join(''));
    if (end > start) spans.push(<mark key={`${start}-${end}`} className="rounded bg-amber-200 px-0.5 text-ink">{chars.slice(start, end).join('')}</mark>);
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

function FileIndexState({ file }) {
  if (file.textStatus === 'ready' && file.textSegments > 0) return null;
  let message = 'Texto ainda não indexado';
  if (file.textStatus === 'failed') message = 'Falha ao ler o texto';
  if (file.textStatus === 'empty') message = file.needsOcr ? 'Sem texto pesquisável; OCR necessário' : 'Sem texto pesquisável';
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
      {query.trim().length > 200 && <p role="alert" className="mt-2 text-sm text-amber-700">A busca usa os primeiros 200 caracteres.</p>}
    </div>

    <section aria-label="Obras" className="rounded-lg bg-white p-5 shadow-sm">
      <h2 className="font-display text-xl text-ink">Obras</h2>
      {!active || (scope ? selected.isLoading : works.isLoading) ? <p className="mt-3 text-sm text-ink-soft">Buscando obras…</p>
        : (scope ? selected.isError : works.isError) ? <p role="alert" className="mt-3 text-sm text-red-700">Não foi possível buscar obras.</p>
          : workItems.length === 0 ? <p className="mt-3 text-sm text-ink-soft">Nenhuma obra encontrada.</p>
            : <ul className="mt-3 space-y-3">{workItems.map((work) => <li key={work.id} className="rounded border border-surface-alt p-3">
              <button type="button" onClick={() => openWork(work.id)} className="font-semibold text-ink hover:text-brand">{work.title}</button>
              <p className="text-sm text-ink-soft">{work.author}</p>
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
      {!active || passages.isLoading ? <p className="mt-3 text-sm text-ink-soft">Buscando passagens…</p>
        : passages.isError ? <p role="alert" className="mt-3 text-sm text-red-700">Não foi possível buscar passagens.</p>
          : !passages.data?.data?.length ? <p className="mt-3 text-sm text-ink-soft">Nenhuma passagem encontrada.</p>
            : <ul className="mt-3 space-y-3">{passages.data.data.map((hit) => <li key={hit.segmentId} className="rounded border border-surface-alt p-3">
              <p className="text-sm font-semibold text-ink">{hit.workTitle} <span className="font-normal text-ink-soft">· {hit.workAuthor}</span></p>
              <p className="text-xs text-ink-soft">{hit.format?.toUpperCase()}{hit.language ? ` · ${hit.language.toUpperCase()}` : ''}{hit.section ? ` · ${hit.section}` : ''} · {hit.origin === 'ocr' ? 'OCR' : 'Texto do arquivo'}</p>
              <p className="mt-2 whitespace-pre-wrap text-sm text-ink"><HighlightedSnippet text={hit.snippet} matches={hit.matches} /></p>
              <div className="mt-2 flex gap-4 text-xs text-brand">
                <button type="button" onClick={() => openBook(hit.workId, hit.fileId, { locator: hit.locator })} className="hover:underline">Abrir neste ponto</button>
                {!scope && <button type="button" onClick={() => chooseScope(hit)} className="hover:underline">Buscar só nesta obra</button>}
              </div>
            </li>)}</ul>}
      {active && <Pager page={passagesPage} hasMore={!!passages.data?.hasMore} onPageChange={setPassagesPage} />}
    </section>

    <section aria-label="Anotações" className="rounded-lg bg-white p-5 shadow-sm">
      <h2 className="font-display text-xl text-ink">Suas anotações</h2>
      {!active || notes.isLoading ? <p className="mt-3 text-sm text-ink-soft">Buscando anotações…</p>
        : notes.isError ? <p role="alert" className="mt-3 text-sm text-red-700">Não foi possível buscar anotações.</p>
          : !notes.data?.data?.length ? <p className="mt-3 text-sm text-ink-soft">Nenhuma anotação encontrada.</p>
            : <ul className="mt-3 space-y-3">{notes.data.data.map((note) => <li key={note.id} className="rounded border border-surface-alt p-3">
              <p className="text-sm font-semibold text-ink">{note.workTitle} <span className="font-normal text-ink-soft">· {note.workAuthor}</span></p>
              {note.quote && <p className="mt-1 text-sm text-ink-soft">“{note.quote}”</p>}
              {note.body && <p className="mt-1 whitespace-pre-wrap text-sm text-ink">{note.body}</p>}
              {note.tags?.length > 0 && <p className="mt-1 text-xs text-ink-soft">{note.tags.join(' · ')}</p>}
              {note.sourceAvailable && note.fileAvailable && note.fileId && note.locator
                ? <button type="button" onClick={() => openBook(note.workId, note.fileId, { locator: note.locator })} className="mt-2 text-xs text-brand hover:underline">Abrir neste ponto</button>
                : <p className="mt-2 text-xs text-ink-soft">{note.sourceAvailable ? 'Anotação sem posição para abrir' : 'Fonte indisponível'}</p>}
            </li>)}</ul>}
      {active && <Pager page={notesPage} hasMore={(notesPage + 1) * PAGE_SIZE < (notes.data?.total ?? 0)} onPageChange={setNotesPage} />}
    </section>
    <p className="text-xs text-ink-soft">A busca textual não relaciona formas como “correr” e “corrida”. PDFs escaneados só aparecem nas passagens após OCR.</p>
  </div>;
}
