import React from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { authenticatedUrl } from '../../../lib/api';
import { useWork } from '../api/useWork';
import { useSetCompletion, useSetWorkFinished } from '../api/useCompletion';
import { completionText, formatSize, languageName, whereYouAre } from '../files';

function FileRow({ file, onRead, onComplete, onReread, busy }) {
  const percent = Math.round(file.percentComplete || 0);
  const started = file.started || percent > 0 || file.completed;
  const usable = file.availability === 'available' && !!file.url;

  return (
    <li className="flex flex-col gap-2 rounded-lg border border-zinc-800 bg-zinc-950 p-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-sm text-zinc-200">
          <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[11px] font-bold uppercase text-zinc-300">{file.format || '?'}</span>
          {file.sizeBytes != null && <span className="text-xs text-zinc-500">{formatSize(file.sizeBytes)}</span>}
          {file.needsOcr && <span className="text-xs text-amber-400">Páginas sem texto (OCR ainda não roda)</span>}
          {file.textStatus === 'ready' && <span className="text-xs text-zinc-500" title="O texto deste arquivo está indexado para a busca">texto indexado</span>}
          {file.textStatus === 'failed' && <span className="text-xs text-amber-400" title="O arquivo abre, mas o texto não pôde ser lido para a busca">texto não lido</span>}
        </div>
        <span className="text-xs text-zinc-400">
          {file.completed ? 'Concluído' : percent > 0 ? `${percent}% lido` : started ? 'Em andamento' : 'Não iniciado'}
        </span>
      </div>

      {started && (
        <div className="h-1 w-full overflow-hidden rounded-full bg-zinc-800">
          <div className="h-full rounded-full bg-blue-500" style={{ width: `${file.completed ? 100 : percent}%` }} />
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        {usable ? (
          <>
            <button
              onClick={() => onRead(file, false)}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500"
            >
              {started && !file.completed ? 'Continuar' : file.completed ? 'Abrir' : 'Ler'}
            </button>
            {file.completed ? (
              <button
                onClick={() => onReread(file)}
                disabled={busy}
                className="rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 disabled:opacity-40"
                title="Começa outra leitura do início; a conclusão anterior continua na contagem"
              >
                Reler
              </button>
            ) : (
              started && (
                <button
                  onClick={() => onRead(file, true)}
                  className="rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
                  title="Abre do início; a sua posição só muda quando você avançar"
                >
                  Do começo
                </button>
              )
            )}
            <button
              onClick={() => onComplete(file, !file.completed)}
              disabled={busy}
              className="ml-auto text-xs text-zinc-500 hover:text-zinc-300 disabled:opacity-40"
              title={file.completed ? 'Volta a contar como em leitura; a posição é mantida' : 'Conta como lido, sem precisar abrir'}
            >
              {file.completed ? 'Reabrir' : 'Marcar como concluído'}
            </button>
            <a
              href={authenticatedUrl(file.url)}
              className="text-xs text-zinc-500 hover:text-zinc-300"
              title="Baixar o arquivo"
            >
              Baixar
            </a>
          </>
        ) : (
          <span className="text-xs text-red-400">
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
    <div className="flex flex-col gap-1 rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-sm" aria-label="Sua leitura">
      {where && <p className="text-zinc-200">{where}</p>}
      {counted && <p className="text-zinc-400">{counted}</p>}
      {work.finished && <p className="text-zinc-400">Você marcou a obra toda como finalizada.</p>}
      <div className="flex gap-3 pt-1 text-xs">
        {work.finished ? (
          <button onClick={() => onFinish(false)} disabled={busy} className="text-blue-400 hover:text-blue-300 disabled:opacity-40">
            Desfazer
          </button>
        ) : (
          work.inProgress && (
            <button onClick={() => onFinish(true)} disabled={busy} className="text-zinc-500 hover:text-zinc-300 disabled:opacity-40">
              Marcar a obra toda como finalizada
            </button>
          )
        )}
      </div>
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
  const { data: work, isLoading, isError } = useWork(workId, { fresh: true });
  const setCompletion = useSetCompletion();
  const setWorkFinished = useSetWorkFinished();

  if (!workId) return null;

  const meta = work?.metadata;
  const editions = work?.editions ?? [];

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm" role="dialog" aria-label="Ficha da obra">
      <div className="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-xl border border-zinc-800 bg-zinc-900 p-6 shadow-2xl">
        <div className="mb-4 flex items-start justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold text-zinc-100">{work?.title ?? 'Carregando…'}</h2>
            {work && <p className="text-sm text-zinc-400">{work.author}</p>}
            {meta?.series && (
              <p className="text-xs text-zinc-500">
                {meta.series}
                {meta.seriesIndex ? ` · ${meta.seriesIndex}` : ''}
              </p>
            )}
          </div>
          <button onClick={closeSheet} className="text-zinc-500 hover:text-zinc-300" aria-label="Fechar">
            ✕
          </button>
        </div>

        {isLoading && <p className="animate-pulse text-sm text-zinc-500">Carregando a obra…</p>}
        {isError && <p className="text-sm text-red-400">Não foi possível abrir esta obra.</p>}

        {work && (
          <div className="flex flex-col gap-5">
            <div className="flex gap-4">
              {work.coverUrl && (
                <img
                  src={authenticatedUrl(work.coverUrl)}
                  alt=""
                  className="h-40 w-28 shrink-0 rounded object-cover"
                  onError={(e) => { e.currentTarget.style.display = 'none'; }}
                />
              )}
              {meta?.description && <p className="line-clamp-6 text-sm leading-relaxed text-zinc-400">{meta.description}</p>}
            </div>

            <ReadingSummary
              work={work}
              busy={setWorkFinished.isPending}
              onFinish={(finished) => setWorkFinished.mutate({ workId: work.id, finished })}
            />

            {editions.length === 0 && <p className="text-sm text-zinc-500">Esta obra ainda não tem arquivos.</p>}
            {editions.map((edition) => {
              const details = [
                languageName(edition.language),
                edition.publisher,
                edition.publicationDate,
                edition.isbn && `ISBN ${edition.isbn}`,
              ].filter(Boolean);
              return (
                <section key={edition.id} className="flex flex-col gap-2" aria-label={`Edição ${edition.id}`}>
                  <div className="flex items-center gap-2">
                    <h3 className="text-sm font-medium text-zinc-200">{details.length ? details.join(' · ') : 'Edição sem detalhes'}</h3>
                    {edition.isPrimary && (
                      <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-zinc-400">Principal</span>
                    )}
                  </div>
                  <ul className="flex flex-col gap-2">
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
  );
}
