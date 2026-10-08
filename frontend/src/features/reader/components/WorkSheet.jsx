import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { authenticatedUrl } from '../../../lib/api';
import { useWork } from '../api/useWork';
import { useSetCompletion, useSetWorkFinished } from '../api/useCompletion';
import { reasonOf, useSplitEdition } from '../api/useVersions';
import { isStaff, useMe } from '../../auth/api/useMe';
import { JoinVersionsDialog } from './JoinVersionsDialog';
import { useCandidates } from '../api/useCandidates';
import { useDialog } from '../../../lib/useDialog';
import { completionText, formatSize, languageName, whereYouAre } from '../files';
import { WorkCover } from '../../../components/ui/WorkCover';
import { sheetNote } from '../../../lib/ocr';
import { fileTextState } from '../../../lib/processing';

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
 * suggested something nobody decided yet, so the sheet itself stays as a reader sees it.
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
        {pending > 0 && <span data-testid="pending-mark" className="absolute right-2 top-2 h-2.5 w-2.5 rounded-full bg-brand ring-2 ring-[#faf8f4]" />}
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

/**
 * The sheet of a work (RF-041, DEC-028): its editions, their languages and the files of each,
 * with the reader's own position in every file, before the reader opens. Each file keeps its
 * own position, so choosing another one continues from *its* place or from the beginning.
 */
export function WorkSheet() {
  const workId = useGlobalStore((state) => state.sheetWorkId);
  const closeSheet = useGlobalStore((state) => state.closeSheet);
  const openBook = useGlobalStore((state) => state.openBook);
  const closeButtonRef = React.useRef(null);
  const dialogRef = React.useRef(null);
  const { data: work, isLoading, isError, error, refetch, isRefetching } = useWork(workId, { fresh: true });
  const setCompletion = useSetCompletion();
  const setWorkFinished = useSetWorkFinished();
  // Putting the files of one book under one work, and taking them out again, is for owner and admin (#37).
  const staff = isStaff(useMe().data);
  const pending = useCandidates(workId, { enabled: staff }).data?.length ?? 0;
  const openWork = useGlobalStore((state) => state.openWork);
  const splitEdition = useSplitEdition();
  const [joining, setJoining] = React.useState(false);
  const [splitting, setSplitting] = React.useState(null); // the edition waiting for a yes
  const [notice, setNotice] = React.useState(null);

  React.useEffect(() => {
    setJoining(false);
    setSplitting(null);
    setNotice(null);
  }, [workId]);

  // An open menu takes the Escape for itself, whichever of the two listeners runs first.
  useDialog(dialogRef, {
    active: !!workId,
    initialFocus: closeButtonRef,
    onEscape: () => {
      if (!dialogRef.current?.querySelector('[role="menu"]')) closeSheet();
    },
  });

  if (!workId) return null;

  const meta = work?.metadata;
  const alternatives = meta?.alternativeTitles ?? [];
  const editions = work?.editions ?? [];
  const files = editions.flatMap((edition) => edition.files);
  const canRead = (file) => file?.availability === 'available' && !!file.url;
  const leadFile = files.find((file) => file.id === work?.continue?.fileId && canRead(file))
    ?? editions.find((edition) => edition.isPrimary)?.files.find(canRead)
    ?? files.find(canRead);
  const canContinue = work?.inProgress && leadFile?.id === work?.continue?.fileId;
  const leadEdition = editions.find((edition) => edition.files.some((file) => file.id === leadFile?.id));
  const leadDetails = [
    leadEdition && languageName(leadEdition.language),
    leadEdition?.publisher,
    leadEdition?.publicationDate,
    leadFile?.format?.toUpperCase(),
  ].filter(Boolean);

  return (
    <div ref={dialogRef} className="fixed inset-0 z-50 flex items-center justify-center bg-ink/60 backdrop-blur-sm sm:p-4" role="dialog" aria-modal="true" aria-label="Ficha da obra">
      <div className="flex h-full w-full flex-col overflow-hidden bg-[#faf8f4] shadow-2xl sm:max-h-[92vh] sm:h-auto sm:max-w-5xl sm:rounded-2xl">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline bg-[#faf8f4] px-4 py-3 sm:px-6">
          <div className="min-w-0">
            <p className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Biblioteca / Ficha da obra</p>
            <h2 className="truncate font-display text-2xl text-ink sm:text-3xl">{work?.title ?? 'Carregando…'}</h2>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {staff && work && <WorkMenu workId={work.id} pending={pending} />}
            <button ref={closeButtonRef} onClick={closeSheet} className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand" aria-label="Fechar">
              ✕
            </button>
          </div>
        </div>

        <div className="min-h-0 overflow-y-auto px-4 py-5 sm:px-6 sm:py-6">
        {isLoading && <p className="animate-pulse text-sm text-ink-faint">Carregando a obra…</p>}
        {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta obra.</LoadError>}

        {work && (
          <div className="flex flex-col gap-7">
            <section className="relative overflow-hidden rounded-2xl bg-surface-alt p-4 shadow-sm sm:p-6">
              <div className="pointer-events-none absolute -right-20 -top-20 h-64 w-64 rounded-full bg-brand/5 blur-3xl" />
              <div className="relative grid gap-5 md:grid-cols-[minmax(170px,230px)_minmax(0,1fr)] md:gap-8">
                <div className="mx-auto w-36 sm:w-44 md:mx-0 md:w-full">
                  <WorkCover item={work} className="aspect-[2/3] w-full rounded-lg object-cover shadow-lg" />
                </div>
                <div className="min-w-0 space-y-4">
                  <div>
                    {meta?.series && (
                      <p className="mb-2 font-mono text-[11px] font-semibold uppercase tracking-widest text-brand">
                        {meta.series}{meta.seriesIndex ? ` · Livro ${meta.seriesIndex}` : ''}
                      </p>
                    )}
                    <h3 className="font-display text-4xl leading-tight text-ink sm:text-5xl">{work.title}</h3>
                    <p className="mt-1 font-body text-sm font-semibold text-brand sm:text-base">{work.author}</p>
                    {alternatives.length > 0 && (
                      <p className="mt-1 text-xs text-ink-soft sm:text-sm">
                        Também conhecida como{' '}
                        {alternatives.map((t, i) => (
                          <React.Fragment key={`${t.id}-${t.title}`}>
                            {i > 0 && ' · '}
                            <span className="text-ink">{t.title}</span>
                            {t.language && <span className="text-ink-faint"> ({languageName(t.language)})</span>}
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
                  {meta?.description && <p className="max-w-2xl whitespace-pre-line text-sm leading-relaxed text-ink-soft sm:text-base">{meta.description}</p>}
                  {!meta?.description && <p className="text-sm text-ink-soft">Escolha uma edição abaixo ou abra o arquivo em que você estava lendo.</p>}
                  {leadFile && (
                    <button
                      onClick={() => openBook(work.id, leadFile.id, { fromStart: false })}
                      className="min-h-11 rounded-lg bg-brand px-5 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-brand-light"
                    >
                      {canContinue ? 'Continuar leitura' : 'Abrir leitor'}
                    </button>
                  )}
                </div>
              </div>
            </section>

            <ReadingSummary
              work={work}
              busy={setWorkFinished.isPending}
              onFinish={(finished) => setWorkFinished.mutate({ workId: work.id, finished })}
            />

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
                edition.publicationDate,
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
          </div>
        )}
        </div>
      </div>
      {joining && work && (
        <JoinVersionsDialog
          work={work}
          onClose={() => setJoining(false)}
          onJoined={(targetId) => {
            setJoining(false);
            openWork(targetId); // this work is retired now: the sheet goes to the one that has its files
          }}
        />
      )}
    </div>
  );
}
