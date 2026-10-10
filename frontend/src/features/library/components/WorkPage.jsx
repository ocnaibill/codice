import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { authenticatedUrl } from '../../../lib/api';
import { useWork } from '../../reader/api/useWork';
import { useWorkSeries } from '../../reader/api/useWorkSeries';
import { useSetCompletion, useSetWorkFinished } from '../../reader/api/useCompletion';
import { useFavoriteToggle } from '../../reader/api/useFavoriteToggle';
import { useFileOutline } from '../../reader/api/useFileOutline';
import { useReadLaterToggle } from '../../reader/api/useReadLaterToggle';
import { WorkOutline } from './WorkOutline';
import { WorkPreview } from './WorkPreview';
import { useFilePreview } from '../../reader/api/useFilePreview';
import { FeaturedQuote, WorkHighlights } from './WorkHighlights';
import { useWorkHighlights } from '../api/useWorkHighlights';
import { reasonOf, useSplitEdition } from '../../reader/api/useVersions';
import { isStaff, useMe } from '../../auth/api/useMe';
import { JoinVersionsDialog } from '../../reader/components/JoinVersionsDialog';
import { useCandidates } from '../../reader/api/useCandidates';
import { completionText, formatSize, languageName, whereYouAre } from '../../reader/files';
import { placeLabel } from '../../reader/placeInWords';
import { WorkCover } from '../../../components/ui/WorkCover';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { ProgressBar } from '../../../components/ui/ProgressBar';
import { sheetNote } from '../../../lib/ocr';
import { fileTextState } from '../../../lib/processing';
import { peopleOf, ROLES } from '../../reader/credits';
import { COMIC_KINDS, unitLabel } from '../../collections/text';
import { otherTitles, titleInUse } from '../../reader/titles';
import { AuthorLinks } from '../../people/components/AuthorLinks';

/**
 * The other names of the work, under the one it goes by (#185): smaller than it, each on a line, with its language when it has one.
 * Three show; the rest are one press away.
 */
function OtherTitles({ titles }) {
  const [all, setAll] = React.useState(false);
  const shown = all ? titles : titles.slice(0, 3);
  return (
    <div aria-label="Outros títulos" className="mt-1.5 flex flex-col">
      {shown.map((t) => (
        <p key={t.title} className="font-display text-xl leading-snug text-ink-soft sm:text-2xl">
          {t.title}
          {t.language && <span className="ml-2 align-middle font-body text-xs text-ink-faint">{languageName(t.language)}</span>}
        </p>
      ))}
      {titles.length > 3 && (
        <button type="button" onClick={() => setAll(!all)} aria-expanded={all} className="mt-1 self-start text-xs text-ink-soft underline decoration-dotted underline-offset-4 hover:text-ink">
          {all ? 'Mostrar menos' : `Mais ${titles.length - 3}`}
        </button>
      )}
    </div>
  );
}

const NOTE_TONE = { ok: 'text-success', warn: 'text-warning', plain: 'text-ink-soft' };

function FileRow({ file, onRead, onComplete, onReread, busy }) {
  const percent = Math.round(file.percentComplete || 0);
  const started = file.started || percent > 0 || file.completed;
  const usable = file.availability === 'available' && !!file.url;
  const ocrNote = sheetNote(file);
  const textState = fileTextState(file, ocrNote); // readable now, searchable when the text has been read (RN-018)

  return (
    <li className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2 text-sm text-ink-soft">
          <span className="rounded bg-brand/10 px-2 py-1 font-mono text-[11px] font-semibold uppercase tracking-wide text-brand">{file.format || '?'}</span>
          {file.sizeBytes != null && <span className="font-mono text-[11px] text-ink-faint">{formatSize(file.sizeBytes)}</span>}
          {ocrNote && <span className={`text-xs ${NOTE_TONE[ocrNote.tone]}`} title={ocrNote.title}>{ocrNote.text}</span>}
          {textState && <span className={`text-xs ${NOTE_TONE[textState.tone]}`} title={textState.title}>{textState.text}</span>}
        </div>
        <span className="shrink-0 font-mono text-[11px] text-ink-soft">
          {file.completed ? 'Concluído' : percent > 0 ? `${percent}% lido` : started ? 'Em andamento' : 'Não iniciado'}
        </span>
      </div>

      {started && (
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-alt">
          <div className="h-full rounded-full bg-brand" style={{ width: `${file.completed ? 100 : percent}%` }} />
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        {usable ? (
          <>
            <button
              onClick={() => onRead(file, false)}
              className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light"
            >
              {started && !file.completed ? 'Continuar' : file.completed ? 'Abrir' : 'Ler'}
            </button>
            {file.completed ? (
              <button
                onClick={() => onReread(file)}
                disabled={busy}
                className="min-h-10 rounded-lg border border-border-hairline bg-surface px-4 py-2 text-xs text-ink hover:bg-surface-alt disabled:opacity-40"
                title="Começa outra leitura do início; a conclusão anterior continua na contagem"
              >
                Reler
              </button>
            ) : (
              started && (
                <button
                  onClick={() => onRead(file, true)}
                  className="min-h-10 rounded-lg border border-border-hairline bg-surface px-4 py-2 text-xs text-ink hover:bg-surface-alt"
                  title="Abre do início; a sua posição só muda quando você avançar"
                >
                  Do começo
                </button>
              )
            )}
            <button
              onClick={() => onComplete(file, !file.completed)}
              disabled={busy}
              className="min-h-10 text-xs text-ink-soft hover:text-brand disabled:opacity-40 sm:ml-auto"
              title={file.completed ? 'Volta a contar como em leitura; a posição é mantida' : 'Conta como lido, sem precisar abrir'}
            >
              {file.completed ? 'Reabrir' : 'Marcar como concluído'}
            </button>
            <a
              href={authenticatedUrl(file.url)}
              className="inline-flex min-h-10 items-center text-xs text-ink-soft hover:text-brand"
              title="Baixar o arquivo"
            >
              Baixar
            </a>
          </>
        ) : (
          <span className="text-xs text-danger">
            {file.availability === 'blocked' ? 'Arquivo bloqueado' : 'Arquivo ausente no servidor'}
          </span>
        )}
      </div>
    </li>
  );
}

/**
 * Where the person is in this work, how many times they finished it, and the mark for the whole
 * work (DEC-079, DEC-080). "You are at 42% in the PDF" is only information: to open another version
 * they choose it below, at its own position or from the start.
 */
function ReadingSummary({ work, busy, onFinish }) {
  const counted = completionText(work.completions);
  const where = work.inProgress ? whereYouAre(work.continue) : null;
  if (!counted && !where && !work.finished) return null;
  return (
    <div className="flex flex-col gap-2 rounded-xl border-l-4 border-brand bg-surface-alt p-4 text-sm" aria-label="Sua leitura">
      <span className="font-mono text-[11px] font-semibold uppercase tracking-widest text-brand">Sua leitura</span>
      {where && <p className="text-ink">{where}</p>}
      {counted && <p className="text-ink-soft">{counted}</p>}
      {work.finished && <p className="text-ink-soft">Você marcou a obra toda como finalizada.</p>}
      <div className="flex gap-3 pt-1 text-xs">
        {work.finished ? (
          <button onClick={() => onFinish(false)} disabled={busy} className="text-brand hover:text-brand-light disabled:opacity-40">
            Desfazer
          </button>
        ) : (
          work.inProgress && (
            <button onClick={() => onFinish(true)} disabled={busy} className="text-ink-soft hover:text-brand disabled:opacity-40">
              Marcar a obra toda como finalizada
            </button>
          )
        )}
      </div>
    </div>
  );
}

/**
 * The actions on a work that are for owner and admin (#70). The dots carry a mark when providers have
 * suggested something nobody decided yet, so the page itself stays as a reader sees it.
 */
function WorkMenu({ workId, pending }) {
  const openMetadata = useGlobalStore((state) => state.openMetadata);
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef(null);

  React.useEffect(() => {
    if (!open) return undefined;
    const onPointer = (event) => {
      if (ref.current && !ref.current.contains(event.target)) setOpen(false);
    };
    const onKey = (event) => {
      if (event.key === 'Escape') {
        event.stopPropagation();
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onPointer);
    window.addEventListener('keydown', onKey, true);
    return () => {
      document.removeEventListener('mousedown', onPointer);
      window.removeEventListener('keydown', onKey, true);
    };
  }, [open]);

  const choose = (tab) => {
    setOpen(false);
    openMetadata(workId, tab);
  };
  const label = pending > 0 ? `Mais ações da obra (${pending} ${pending === 1 ? 'sugestão' : 'sugestões'})` : 'Mais ações da obra';
  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        className="relative flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl leading-none text-ink-soft hover:bg-surface-alt hover:text-brand"
      >
        ⋯
        {pending > 0 && <span data-testid="pending-mark" className="absolute right-2 top-2 h-2.5 w-2.5 rounded-full bg-brand ring-2 ring-surface" />}
      </button>
      {open && (
        <div role="menu" className="absolute right-0 top-full z-10 mt-1 w-60 overflow-hidden rounded-xl border border-border-hairline bg-white py-1 shadow-lg">
          <button role="menuitem" onClick={() => choose('suggestions')} className="flex min-h-11 w-full items-center justify-between gap-3 px-4 py-2 text-left text-sm text-ink hover:bg-surface-alt">
            Sugestões dos provedores
            {pending > 0 && <span className="rounded-full bg-brand px-2 py-0.5 font-mono text-[10px] text-white">{pending}</span>}
          </button>
          <button role="menuitem" onClick={() => choose('edit')} className="flex min-h-11 w-full items-center px-4 py-2 text-left text-sm text-ink hover:bg-surface-alt">
            Editar metadados
          </button>
        </div>
      )}
    </div>
  );
}

/** The people of a role on the work, each opening their page; the name is the one the account is shown. */
function PeopleLinks({ people }) {
  return <AuthorLinks authors={people.map((c) => ({ id: c.personId, name: c.displayName || c.name }))} />;
}

/** "2018-05-25T03:00:00+00:00" is a year to a reader; a date that does not start with one is said as it was kept. */
function yearOf(date) {
  const year = /^(\d{4})(?:-|T|$)/.exec(date || '');
  return year ? year[1] : date;
}

const canRead = (file) => file?.availability === 'available' && !!file.url;
// The formats the text index has a table of contents for.
const OUTLINED_FORMATS = new Set(['epub', 'pdf', 'txt', 'md']);
// The ones a plain preview of the text makes sense for: a PDF keeps its pages, and is read in its own viewer.
const PREVIEW_FORMATS = new Set(['epub', 'txt', 'md']);

/** The file the page leads with: the one the person was reading if it still opens, else the primary edition's first, else any. */
function leadFileOf(work) {
  const editions = work?.editions ?? [];
  const files = editions.flatMap((edition) => edition.files);
  return files.find((file) => file.id === work?.continue?.fileId && canRead(file))
    ?? editions.find((edition) => edition.isPrimary)?.files.find(canRead)
    ?? files.find(canRead);
}

/** The synopsis, as many lines as fit and the rest one press away (the long ones of a publisher fill the page otherwise). */
function Description({ text }) {
  const [all, setAll] = React.useState(false);
  const long = text.length > 480;
  return (
    <div className="max-w-2xl">
      <p
        className="whitespace-pre-line text-sm leading-relaxed text-ink-soft sm:text-base"
        style={long && !all ? { display: '-webkit-box', WebkitLineClamp: 6, WebkitBoxOrient: 'vertical', overflow: 'hidden' } : undefined}
      >
        {text}
      </p>
      {long && (
        <button type="button" onClick={() => setAll(!all)} aria-expanded={all} className="mt-1 text-xs text-ink-soft underline decoration-dotted underline-offset-4 hover:text-ink">
          {all ? 'Mostrar menos' : 'Ler mais'}
        </button>
      )}
    </div>
  );
}

/** The heart of the work: to keep it among the favorites (#179), the same as on its card. */
function FavoriteHeart({ work }) {
  const toggle = useFavoriteToggle(work.id);
  const label = work.isFavorite ? 'Remover dos favoritos' : 'Adicionar aos favoritos';
  return (
    <button
      type="button"
      className="library-favorite library-favorite--page"
      onClick={() => toggle.mutate(!work.isFavorite)}
      disabled={toggle.isPending}
      aria-pressed={!!work.isFavorite}
      aria-label={label}
      title={label}
    >
      <LibraryIcon name="heart" />
    </button>
  );
}

/** "Ler depois": the work goes to a list the Códice keeps for the person (DEC-152), beside the heart that makes it a favorite. */
function ReadLaterButton({ work }) {
  const toggle = useReadLaterToggle(work.id);
  const label = work.readLater ? 'Tirar de "Ler depois"' : 'Ler depois';
  return (
    <button
      type="button"
      className="library-favorite library-favorite--page"
      onClick={() => toggle.mutate(!work.readLater)}
      disabled={toggle.isPending}
      aria-pressed={!!work.readLater}
      aria-label={label}
      title={label}
    >
      <LibraryIcon name="bookmark" />
    </button>
  );
}

/**
 * The page of a work (RF-041, DEC-028, DEC-149): what it is, where the person is in it, and its editions and the files of each,
 * with the reader's own position in every file, before the reader opens. Each file keeps its own position, so choosing another
 * one continues from *its* place or from the beginning. It takes the place of the page it was opened from, which is still
 * there when the person goes back.
 */
export function WorkPage() {
  const workId = useGlobalStore((state) => state.sheetWorkId);
  const closeSheet = useGlobalStore((state) => state.closeSheet);
  const openBook = useGlobalStore((state) => state.openBook);
  const setView = useGlobalStore((state) => state.setLibraryView);
  const openCategory = useGlobalStore((state) => state.openCategory);
  const pageRef = React.useRef(null);
  const headingRef = React.useRef(null);
  const { data: work, isLoading, isError, error, refetch, isRefetching } = useWork(workId, { fresh: true });
  const setCompletion = useSetCompletion();
  const setWorkFinished = useSetWorkFinished();
  // Putting the files of one book under one work, and taking them out again, is for owner and admin (#37).
  const staff = isStaff(useMe().data);
  const pending = useCandidates(workId, { enabled: staff }).data?.length ?? 0;
  const openWork = useGlobalStore((state) => state.openWork);
  const openCollection = useGlobalStore((state) => state.openCollection);
  // The official collection of the series the work is in, to open it from the name of the series (#187).
  const seriesCollection = useWorkSeries(workId).data?.collection ?? null;
  const leadFile = work ? leadFileOf(work) : undefined;
  const notes = useWorkHighlights(workId).data?.data;
  const [previewFrom, setPreviewFrom] = React.useState(null); // null: the window that holds the person's place
  const previewing = useFilePreview(leadFile?.id, { from: previewFrom ?? undefined, enabled: !!leadFile && PREVIEW_FORMATS.has((leadFile.format || '').toLowerCase()) });
  const outline = useFileOutline(leadFile?.id, { enabled: !!leadFile && OUTLINED_FORMATS.has((leadFile.format || '').toLowerCase()) }).data;
  const splitEdition = useSplitEdition();
  const [joining, setJoining] = React.useState(false);
  const [splitting, setSplitting] = React.useState(null); // the edition waiting for a yes
  const [notice, setNotice] = React.useState(null);

  React.useEffect(() => {
    setJoining(false);
    setSplitting(null);
    setNotice(null);
    setPreviewFrom(null);
  }, [workId]);

  // A page that opens starts at its top, with the focus on what it is about (a screen reader says it).
  React.useEffect(() => {
    if (!workId) return;
    pageRef.current?.scrollIntoView?.({ block: 'start' });
    headingRef.current?.focus?.({ preventScroll: true });
  }, [workId, !!work]);

  if (!workId) return null;

  const meta = work?.metadata;
  const authors = peopleOf(meta?.contributors, 'author');
  const otherRoles = ROLES.filter((r) => r.key !== 'author').map((r) => ({ ...r, people: peopleOf(meta?.contributors, r.key) })).filter((r) => r.people.length > 0);
  const editions = work?.editions ?? [];
  const files = editions.flatMap((edition) => edition.files);
  const canContinue = work?.inProgress && leadFile?.id === work?.continue?.fileId;
  const leadEdition = editions.find((edition) => edition.files.some((file) => file.id === leadFile?.id));
  // The work goes by the title written for the edition in focus, if there is one; its other names go under it.
  const shownTitle = work ? titleInUse(work, leadEdition) : '';
  const others = work ? otherTitles(work, shownTitle) : [];
  const leadDetails = [
    COMIC_KINDS.find((k) => k.key === meta?.comicKind)?.one,
    leadEdition && languageName(leadEdition.language),
    leadEdition?.publisher,
    yearOf(leadEdition?.publicationDate),
    leadFile?.format?.toUpperCase(),
  ].filter(Boolean);
  const category = meta?.categories?.[0] ?? null;
  const openNote = (note) => openBook(work.id, note.fileId, { locator: note.locator, context: { kind: 'note', quote: note.quote } });
  // Where the person stopped, in the file that counts: the chapter and the place in it (DEC-148), and how far through the file.
  const last = work?.continue ?? null;
  const started = !!last && (last.percentComplete > 0 || last.completed || !!last.position);
  const resumeAt = canContinue && last ? placeLabel((last.format || '').toLowerCase(), last, last.position) : null;
  const formatsText = work?.formatCount > 1 ? `${work.formatCount} formatos disponíveis` : null;

  return (
    <div ref={pageRef} className="library-dashboard" role="region" aria-label="Ficha da obra">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <nav aria-label="Onde você está" className="flex min-w-0 flex-wrap items-center gap-2 text-[13px] text-ink-soft">
          <button className="library-button" onClick={closeSheet}>← Voltar</button>
          <button className="library-text-link" style={{ marginLeft: 0 }} onClick={() => setView('all')}>Biblioteca</button>
          {category && (
            <>
              <span aria-hidden="true">›</span>
              <button className="library-text-link" style={{ marginLeft: 0 }} onClick={() => openCategory(category.id)}>{category.path || category.name}</button>
            </>
          )}
          <span aria-hidden="true">›</span>
          <span aria-current="page" className="min-w-0 truncate text-ink">{shownTitle || 'Carregando…'}</span>
        </nav>
        {staff && work && <WorkMenu workId={work.id} pending={pending} />}
      </div>

      {isLoading && <p className="animate-pulse py-6 text-sm text-ink-faint">Carregando a obra…</p>}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta obra.</LoadError>}

      {work && (
        <div className="mt-4 flex flex-col gap-7">
          <section className="relative overflow-hidden rounded-2xl bg-surface-alt p-4 shadow-sm sm:p-6">
            <div className="pointer-events-none absolute -right-20 -top-20 h-64 w-64 rounded-full bg-brand/5 blur-3xl" />
            <div className="relative grid gap-5 md:grid-cols-[minmax(170px,230px)_minmax(0,1fr)] md:gap-8">
              <div className="mx-auto flex w-36 flex-col gap-3 sm:w-44 md:mx-0 md:w-full">
                <div className="relative">
                  <WorkCover item={work} className="aspect-[2/3] w-full rounded-lg object-cover shadow-lg" />
                  {formatsText && (
                    <span className="absolute left-1.5 top-1.5 rounded-sm bg-ink/90 px-1.5 py-1 font-mono text-[9px] uppercase text-white">{formatsText}</span>
                  )}
                </div>
                {started && (
                  <div className="flex flex-col gap-1" aria-label="Seu progresso">
                    <ProgressBar percent={last.completed ? 100 : last.percentComplete} color="brand" />
                    <p className="font-mono text-[11px] text-ink-soft">
                      {last.completed ? 'Concluído' : `${Math.round(last.percentComplete || 0)}% lido`}
                      {resumeAt ? ` · ${resumeAt}` : ''}
                    </p>
                  </div>
                )}
              </div>
              <div className="min-w-0 space-y-4">
                <div>
                  {meta?.series && (
                    <p className="mb-2 font-mono text-[11px] font-semibold uppercase tracking-widest text-brand">
                      {seriesCollection ? (
                        <button
                          type="button"
                          onClick={() => openCollection(seriesCollection.id)}
                          title="Ver as outras obras da série"
                          className="font-semibold uppercase tracking-widest underline decoration-dotted decoration-brand/60 underline-offset-4 hover:decoration-solid"
                        >
                          {meta.series}
                        </button>
                      ) : (
                        meta.series
                      )}
                      {meta.unit ? ` · ${unitLabel(meta.unit, meta.seriesIndex || null)}` : meta.seriesIndex ? ` · Livro ${meta.seriesIndex}` : ''}
                    </p>
                  )}
                  <h1 ref={headingRef} tabIndex={-1} className="font-display text-4xl leading-tight text-ink outline-none sm:text-5xl">{shownTitle}</h1>
                  {others.length > 0 && <OtherTitles key={work.id} titles={others} />}
                  <p className="mt-1 font-body text-sm font-semibold text-brand sm:text-base">
                    {authors.length > 0 ? <PeopleLinks people={authors} /> : work.author}
                  </p>
                  {otherRoles.length > 0 && (
                    <p className="mt-1 text-xs text-ink-soft sm:text-sm">
                      {otherRoles.map((r, i) => (
                        <React.Fragment key={r.key}>
                          {i > 0 && ' · '}
                          {r.heading}: <PeopleLinks people={r.people} />
                        </React.Fragment>
                      ))}
                    </p>
                  )}
                </div>
                {leadDetails.length > 0 && (
                  <div className="flex flex-wrap gap-2" aria-label="Dados da edição em foco">
                    {leadDetails.map((detail, index) => (
                      <span key={`${detail}-${index}`} className="rounded-full bg-white px-3 py-1 font-mono text-[11px] text-ink-soft shadow-sm">{detail}</span>
                    ))}
                  </div>
                )}
                {work.tags?.length > 0 && (
                  <ul className="flex flex-wrap gap-x-3 gap-y-1" aria-label="Etiquetas">
                    {work.tags.map((tag) => (
                      <li key={tag} className="font-mono text-[11px] text-ink-soft">#{tag}</li>
                    ))}
                  </ul>
                )}
                <FeaturedQuote notes={notes} workId={work.id} onOpen={openNote} />
                {meta?.description ? <Description text={meta.description} /> : <p className="text-sm text-ink-soft">Escolha uma edição abaixo ou abra o arquivo em que você estava lendo.</p>}
                <div className="flex flex-wrap items-center gap-3">
                  {leadFile && (
                    <button
                      onClick={() => openBook(work.id, leadFile.id, { fromStart: false })}
                      className="min-h-11 rounded-lg bg-brand px-5 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-brand-light"
                    >
                      {canContinue ? 'Continuar leitura' : leadFile.format ? `Abrir leitor ${leadFile.format.toUpperCase()}` : 'Abrir leitor'}
                    </button>
                  )}
                  <FavoriteHeart work={work} />
                  <ReadLaterButton work={work} />
                </div>
                {canContinue && last?.chapter && (
                  <p className="text-sm text-ink-soft">
                    <span className="font-semibold text-ink">Retomando:</span> {last.chapter}
                  </p>
                )}
              </div>
            </div>
          </section>

          <ReadingSummary
            work={work}
            busy={setWorkFinished.isPending}
            onFinish={(finished) => setWorkFinished.mutate({ workId: work.id, finished })}
          />

          {leadFile && PREVIEW_FORMATS.has((leadFile.format || '').toLowerCase()) && (
            <WorkPreview
              title={shownTitle}
              data={previewing.data}
              busy={previewing.isFetching}
              onPrev={setPreviewFrom}
              onNext={setPreviewFrom}
              onOpen={(segment) => openBook(work.id, leadFile.id, { locator: segment.locator, context: { kind: 'chapter', quote: segment.chapter || shownTitle } })}
            />
          )}

          {leadFile && (
            <WorkOutline
              outline={outline}
              onOpen={(locator, title) => openBook(work.id, leadFile.id, { locator, context: { kind: 'chapter', quote: title } })}
            />
          )}
          {notice && (
            <div role="status" className="flex flex-wrap items-center gap-3 rounded-xl border-l-4 border-success bg-surface-alt p-4 text-sm text-ink">
              <span>{notice.text}</span>
              {notice.workId && (
                <button onClick={() => openWork(notice.workId)} className="font-semibold text-brand hover:text-brand-light">
                  Abrir essa obra
                </button>
              )}
            </div>
          )}

          <div className="flex flex-wrap items-end justify-between gap-3">
            <div>
              <p className="font-mono text-[11px] font-semibold uppercase tracking-widest text-ink-faint">Acervo digital</p>
              <h3 className="font-display text-2xl text-ink sm:text-3xl">Edições e arquivos</h3>
            </div>
            {staff && (
              <button
                onClick={() => setJoining(true)}
                className="min-h-10 rounded-lg border border-border-hairline bg-white px-4 py-2 text-xs text-ink hover:bg-surface-alt"
                title="Esta obra é outra versão de um livro que já está no acervo: os arquivos ficam todos sob uma obra só"
              >
                Juntar com outra obra…
              </button>
            )}
          </div>
          {editions.length === 0 && <p className="text-sm text-ink-faint">Esta obra ainda não tem arquivos.</p>}
          {editions.map((edition) => {
            const details = [
              languageName(edition.language),
              edition.publisher,
              yearOf(edition.publicationDate),
              edition.isbn && `ISBN ${edition.isbn}`,
            ].filter(Boolean);
            return (
              <section key={edition.id} className="flex flex-col gap-3" aria-label={`Edição ${edition.id}`}>
                <div className="flex flex-wrap items-center gap-2">
                  <h4 className="text-sm font-semibold text-ink">{details.length ? details.join(' · ') : 'Edição sem detalhes'}</h4>
                  {edition.isPrimary && (
                    <span className="rounded bg-success/10 px-2 py-1 font-mono text-[10px] uppercase tracking-wide text-success">Principal</span>
                  )}
                  {staff && editions.length > 1 && splitting !== edition.id && (
                    <button
                      onClick={() => setSplitting(edition.id)}
                      className="ml-auto min-h-10 text-xs text-ink-soft hover:text-brand"
                      title="Tira esta edição desta obra: volta para a obra de onde veio, ou vira uma obra nova"
                    >
                      Separar em obra própria
                    </button>
                  )}
                </div>
                {splitting === edition.id && (
                  <div className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 text-sm text-ink" aria-label="Confirmar a separação">
                    <p>
                      Separar esta edição? Ela volta para a obra de onde veio, se houver, ou vira uma obra nova.
                      As notas e as conclusões dos arquivos dela vão junto.
                    </p>
                    <div className="flex flex-wrap gap-2">
                      <button
                        disabled={splitEdition.isPending}
                        onClick={() =>
                          splitEdition.mutate(
                            { editionId: edition.id },
                            {
                              onSuccess: (result) => {
                                setSplitting(null);
                                setNotice({
                                  workId: result.workId,
                                  text: result.restored ? 'Edição separada: ela voltou para a obra de onde veio.' : 'Edição separada em uma obra nova.',
                                });
                              },
                              onError: (error) => {
                                setSplitting(null);
                                setNotice({ text: reasonOf(error, 'Não foi possível separar a edição.') });
                              },
                            }
                          )
                        }
                        className="min-h-10 rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40"
                      >
                        Separar
                      </button>
                      <button onClick={() => setSplitting(null)} className="min-h-10 px-4 py-2 text-xs text-ink-soft hover:text-brand">
                        Cancelar
                      </button>
                    </div>
                  </div>
                )}
                <ul className="flex flex-col gap-3">
                  {edition.files.map((file) => (
                    <FileRow
                      key={file.id}
                      file={file}
                      busy={setCompletion.isPending}
                      onRead={(f, fromStart) => openBook(work.id, f.id, { fromStart })}
                      onComplete={(f, completed) => setCompletion.mutate({ fileId: f.id, completed })}
                      onReread={(f) =>
                        setCompletion.mutate(
                          { fileId: f.id, completed: false, restart: true },
                          { onSuccess: () => openBook(work.id, f.id, { fromStart: true }) }
                        )
                      }
                    />
                  ))}
                </ul>
              </section>
            );
          })}

          <WorkHighlights notes={notes} onOpen={openNote} />
        </div>
      )}
      {joining && work && (
        <JoinVersionsDialog
          work={work}
          onClose={() => setJoining(false)}
          onJoined={(targetId) => {
            setJoining(false);
            openWork(targetId); // this work is retired now: the page goes to the one that has its files
          }}
        />
      )}
    </div>
  );
}
