import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Shelf } from '../../home/components/Shelf';
import { formatCount, formatReadingTime } from '../../home/utils/format';
import { useWorks } from '../../library/api/useWorks';
import { ROLES } from '../../reader/credits';
import { usePerson } from '../api/usePerson';
import { usePersonHighlights } from '../api/usePersonHighlights';
import { FeaturedQuote, WorkHighlights } from '../../library/components/WorkHighlights';
import { isStaff, useMe } from '../../auth/api/useMe';
import { PersonProfile } from './PersonProfile';

// A person's works come in one request of this many: a person with more is paged, and the series shelves are of the page.
const PAGE_SIZE = 100;

const ROLE_NAMES = Object.fromEntries(ROLES.map((r) => [r.key, r]));

const keyOf = (text) => String(text || '').normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/\s+/g, ' ').trim();

/**
 * The works of a person, in the shelves they have: one for each series (in the order of the series), and "Demais obras" for those that are in
 * none. A work is in the series its metadata says (DEC-130).
 */
export function shelvesOf(works) {
  const bySeries = new Map();
  const loose = [];
  for (const work of works) {
    const series = (work.series || '').trim();
    if (!series) {
      loose.push(work);
      continue;
    }
    const key = keyOf(series);
    if (!bySeries.has(key)) bySeries.set(key, { key, name: series, works: [] });
    bySeries.get(key).works.push(work);
  }
  const series = [...bySeries.values()].sort((a, b) => a.name.localeCompare(b.name, 'pt'));
  for (const s of series) {
    // By the number in the series; the ones with none (0) after, by title.
    s.works.sort((a, b) => (a.seriesIndex || Infinity) - (b.seriesIndex || Infinity) || String(a.title).localeCompare(String(b.title), 'pt'));
  }
  return { series, loose };
}

/** For staff: the names of the people are corrected in the administration, in the tab of the duplicates (DEC-093). */
function CorrectNameButton() {
  const staff = isStaff(useMe().data);
  const openAdmin = useGlobalStore((state) => state.openAdmin);
  const setAdminTab = useGlobalStore((state) => state.setAdminTab);
  if (!staff) return null;
  return (
    <button
      type="button"
      className="library-button"
      title="O nome e as formas em que ele aparece se corrigem na administração"
      onClick={() => {
        openAdmin();
        setAdminTab('duplicates');
      }}
    >
      Corrigir o nome
    </button>
  );
}

/** The works of the page that match what was typed (in the title or the author) and the format chosen (its primary file's). */
export function filterWorks(works, { text = '', format = '' } = {}) {
  const wanted = keyOf(text);
  return works.filter((work) => {
    if (format && String(work.format || '').toLowerCase() !== format) return false;
    if (wanted && !keyOf(`${work.title} ${work.author || ''}`).includes(wanted)) return false;
    return true;
  });
}

/** In chronological order: by the year the work was first published, those nobody dated last, by title. */
export function byYear(works) {
  return [...works].sort(
    (a, b) =>
      (a.originalYear ?? Infinity) - (b.originalYear ?? Infinity) || String(a.title).localeCompare(String(b.title), 'pt')
  );
}

function Stat({ label, value }) {
  return (
    <div className="library-stat">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

/**
 * The page of a person (#186, DEC-157): who they are (what Wikidata and Wikipedia say), what the library has of theirs and how far the
 * person who looks is in it, the themes of their works, and the works in shelves, one for each series and the rest apart. It takes the place
 * of the page it was opened from; a work opened from here has its own page, and going back finds this one.
 */
export function PersonPage() {
  const id = useGlobalStore((state) => state.personSheetId);
  const close = useGlobalStore((state) => state.closePerson);
  const openCollection = useGlobalStore((state) => state.openCollection);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const openBook = useGlobalStore((state) => state.openBook);
  const headingRef = React.useRef(null);
  const pageRef = React.useRef(null);
  const [chosen, setChosen] = React.useState(null);
  const [page, setPage] = React.useState(1);
  const [text, setText] = React.useState('');
  const [format, setFormat] = React.useState('');
  const [order, setOrder] = React.useState('series'); // 'series' (a shelf for each) or 'year' (one, in chronological order)
  const { data: person, isLoading, isError, error, refetch, isRefetching } = usePerson(id);
  const notes = usePersonHighlights(id).data?.data;

  // Another person, or another role, starts at the first page.
  React.useEffect(() => {
    setChosen(null);
    setPage(1);
    setText('');
    setFormat('');
    setOrder('series');
  }, [id]);

  const roles = person?.roles ?? [];
  const role = chosen && roles.some((r) => r.role === chosen) ? chosen : roles[0]?.role;
  const works = useWorks({ person: id ?? undefined, role, sort: 'title', limit: PAGE_SIZE, page, enabled: !!id && !!role });

  // A page that opens starts at its top, with the focus on who it is about.
  React.useEffect(() => {
    if (!id) return;
    pageRef.current?.scrollIntoView?.({ block: 'start' });
    headingRef.current?.focus?.({ preventScroll: true });
  }, [id, !!person]);

  if (!id) return null;

  const totalPages = works.data?.totalPages ?? 1;
  const pick = (key) => {
    setChosen(key);
    setPage(1);
  };
  const loaded = works.data?.data ?? [];
  const formats = [...loaded.reduce((m, w) => {
    const f = String(w.format || '').toLowerCase();
    return f ? m.set(f, (m.get(f) ?? 0) + 1) : m;
  }, new Map())].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const shown = filterWorks(loaded, { text, format });
  const filtering = text.trim() !== '' || format !== '';
  const { series, loose } = shelvesOf(shown);
  const collectionOf = (name) => (person?.collections ?? []).find((c) => keyOf(c.name) === keyOf(name));
  const stats = person?.stats;
  const tags = person?.tags ?? [];
  const openNote = (note) => openBook(note.workId, note.fileId, { locator: note.locator, context: { kind: 'note', quote: note.quote } });

  return (
    <div ref={pageRef} className="library-dashboard" role="region" aria-label="Pessoa">
      <nav aria-label="Onde você está" className="flex flex-wrap items-center gap-2 text-[13px] text-ink-soft">
        <button className="library-button" onClick={close}>← Voltar</button>
        <button className="library-text-link" style={{ marginLeft: 0 }} onClick={() => setView('all')}>Biblioteca</button>
        <span aria-hidden="true">›</span>
        <span className="text-ink-faint">Pessoa</span>
        <span aria-hidden="true">›</span>
        <span aria-current="page" className="min-w-0 truncate text-ink">{person?.displayName ?? 'Carregando…'}</span>
      </nav>

      {isLoading && <p className="animate-pulse py-6 text-sm text-ink-faint">Carregando a pessoa…</p>}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta página.</LoadError>}
      {person && (
        <div className="mt-4 flex flex-col gap-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <h1 ref={headingRef} tabIndex={-1} className="font-display text-4xl leading-tight text-ink outline-none sm:text-5xl">{person.displayName}</h1>
            <CorrectNameButton />
          </div>
          <PersonProfile person={person} />

          {person.aliases.length > 0 && (
            <p className="text-sm text-ink-soft">
              Também aparece como <span className="text-ink">{person.aliases.join(' · ')}</span>
            </p>
          )}

          {stats && stats.works > 0 && (
            <dl aria-label="Números" className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Stat label="Obras no acervo" value={formatCount(stats.works)} />
              <Stat label="Que você terminou" value={formatCount(stats.finished)} />
              <Stat label="Em andamento" value={formatCount(stats.inProgress)} />
              <Stat label="Horas lidas" value={stats.readingSeconds > 0 ? formatReadingTime(stats.readingSeconds) : '—'} />
            </dl>
          )}

          {tags.length > 0 && (
            <ul aria-label="Temas" className="flex flex-wrap gap-x-3 gap-y-1">
              {tags.map((tag) => (
                <li key={tag.name} className="font-mono text-[11px] text-ink-soft" title={`${tag.works} ${tag.works === 1 ? 'obra' : 'obras'}`}>#{tag.name}</li>
              ))}
            </ul>
          )}

          <FeaturedQuote notes={notes} workId={id} showWork onOpen={openNote} />

          {roles.length === 0 ? (
            <p className="rounded-lg border border-dashed border-surface-alt bg-surface/50 px-4 py-8 text-center text-sm text-ink-faint">
              O acervo não tem obra dessa pessoa.
            </p>
          ) : (
            <>
              {roles.length > 1 && (
                <div role="group" aria-label="Função na obra" className="flex flex-wrap gap-2">
                  {roles.map((r) => (
                    <button
                      key={r.role}
                      onClick={() => pick(r.role)}
                      aria-pressed={r.role === role}
                      className="min-h-10 rounded-full border border-border-hairline bg-white px-4 text-sm text-ink hover:bg-surface-alt aria-pressed:border-brand aria-pressed:bg-brand aria-pressed:text-white"
                    >
                      {ROLE_NAMES[r.role]?.one ?? r.role}
                      <span className="ml-2 font-mono text-[11px] opacity-70">{r.works}</span>
                    </button>
                  ))}
                </div>
              )}

              {loaded.length > 1 && (
                <div className="flex flex-wrap items-center gap-3" role="search" aria-label="Filtrar as obras">
                  <input
                    type="search"
                    value={text}
                    onChange={(e) => setText(e.target.value)}
                    placeholder="Filtrar as obras desta pessoa…"
                    aria-label="Filtrar as obras desta pessoa"
                    className="min-h-10 min-w-[220px] flex-1 rounded-lg border border-border-hairline bg-white px-3 text-sm text-ink outline-none focus:border-brand"
                  />
                  {formats.length > 1 && (
                    <div role="group" aria-label="Formato" className="flex flex-wrap gap-2">
                      <button type="button" aria-pressed={format === ''} onClick={() => setFormat('')} className="min-h-9 rounded-full border border-border-hairline bg-white px-3 text-xs text-ink aria-pressed:border-brand aria-pressed:bg-brand aria-pressed:text-white">
                        Todos
                      </button>
                      {formats.map(([f, n]) => (
                        <button key={f} type="button" aria-pressed={format === f} onClick={() => setFormat(format === f ? '' : f)} className="min-h-9 rounded-full border border-border-hairline bg-white px-3 text-xs text-ink aria-pressed:border-brand aria-pressed:bg-brand aria-pressed:text-white">
                          {f.toUpperCase()} <span className="font-mono opacity-70">({n})</span>
                        </button>
                      ))}
                    </div>
                  )}
                  <label className="flex items-center gap-2 text-xs text-ink-soft">
                    Ordem
                    <select value={order} onChange={(e) => setOrder(e.target.value)} className="min-h-9 rounded-lg border border-border-hairline bg-white px-2 text-sm text-ink">
                      <option value="series">Por série</option>
                      <option value="year">Cronológica (ano da primeira publicação)</option>
                    </select>
                  </label>
                </div>
              )}

              {works.isError ? (
                <LoadError error={works.error} onRetry={works.refetch} retrying={works.isRefetching}>Não foi possível carregar as obras.</LoadError>
              ) : filtering && shown.length === 0 && !works.isLoading ? (
                <p className="rounded-lg border border-dashed border-surface-alt bg-surface/50 px-4 py-8 text-center text-sm text-ink-faint">
                  Nenhuma obra desta pessoa tem isso.
                </p>
              ) : order === 'year' ? (
                <Shelf headingId="person-chronological" title="Em ordem cronológica" total={shown.length} items={byYear(shown)} isLoading={works.isLoading} />
              ) : (
                <>
                  {series.map((s, i) => {
                    const collection = collectionOf(s.name);
                    return (
                      <Shelf
                        key={s.key}
                        headingId={`person-series-${i}`}
                        title={s.name}
                        total={s.works.length}
                        countWord={s.works.length === 1 ? 'volume no acervo' : 'volumes no acervo'}
                        items={s.works}
                        isLoading={false}
                        seeAll={collection ? { label: 'Ver a coleção', onClick: () => openCollection(collection.id) } : undefined}
                      />
                    );
                  })}
                  {(loose.length > 0 || works.isLoading) && (
                    <Shelf
                      headingId="person-loose"
                      title={series.length > 0 ? 'Demais obras' : role === 'author' || !ROLE_NAMES[role] ? 'Obras' : ROLE_NAMES[role].heading}
                      total={works.isLoading ? undefined : loose.length}
                      items={loose}
                      isLoading={works.isLoading}
                    />
                  )}
                </>
              )}
              {totalPages > 1 && (
                <nav className="library-pagination" aria-label="Páginas das obras da pessoa">
                  <button className="library-button" disabled={page <= 1 || works.isFetching} onClick={() => setPage(page - 1)}>Anterior</button>
                  <span aria-live="polite">{page} de {totalPages}</span>
                  <button className="library-button" disabled={page >= totalPages || works.isFetching} onClick={() => setPage(page + 1)}>Próxima</button>
                </nav>
              )}
            </>
          )}

          <WorkHighlights notes={notes} onOpen={openNote} title="Seus destaques nas obras dele" showWork />
        </div>
      )}
    </div>
  );
}
