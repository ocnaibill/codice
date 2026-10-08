import React from 'react';
import { LoadError } from '../../../components/ui/LoadError';
import { useDialog } from '../../../lib/useDialog';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { LibraryGrid } from '../../home/components/LibraryGrid';
import { useWorks } from '../../library/api/useWorks';
import { ROLES } from '../../reader/credits';
import { usePerson } from '../api/usePerson';

const PAGE_SIZE = 12;

const ROLE_NAMES = Object.fromEntries(ROLES.map((r) => [r.key, r]));

/**
 * The page of a person (#186): the works the library has of theirs, by the role they have in them (as author first, then the other
 * functions), the other names they are written with, and the collections those works are in. It opens from a name on the sheet of a
 * work; a work opened from here opens its own sheet over this page, and closing it comes back.
 */
export function PersonSheet() {
  const id = useGlobalStore((state) => state.personSheetId);
  const close = useGlobalStore((state) => state.closePerson);
  const openCollection = useGlobalStore((state) => state.openCollection);
  const dialogRef = React.useRef(null);
  const closeRef = React.useRef(null);
  const [chosen, setChosen] = React.useState(null);
  const [page, setPage] = React.useState(1);
  const { data: person, isLoading, isError, error, refetch, isRefetching } = usePerson(id);

  // Another person, or another role, starts at the first page.
  React.useEffect(() => {
    setChosen(null);
    setPage(1);
  }, [id]);

  const roles = person?.roles ?? [];
  const role = chosen && roles.some((r) => r.role === chosen) ? chosen : roles[0]?.role;
  const works = useWorks({ person: id ?? undefined, role, sort: 'title', limit: PAGE_SIZE, page, enabled: !!id && !!role });

  useDialog(dialogRef, { active: !!id, initialFocus: closeRef, onEscape: close });

  if (!id) return null;

  const totalPages = works.data?.totalPages ?? 1;
  const pick = (key) => {
    setChosen(key);
    setPage(1);
  };
  const toCollection = (collectionId) => {
    close();
    openCollection(collectionId);
  };

  return (
    <div ref={dialogRef} className="fixed inset-0 z-50 flex items-center justify-center bg-ink/60 backdrop-blur-sm sm:p-4" role="dialog" aria-modal="true" aria-label="Pessoa">
      <div className="flex h-full w-full flex-col overflow-hidden bg-[#faf8f4] shadow-2xl sm:max-h-[92vh] sm:h-auto sm:max-w-5xl sm:rounded-2xl">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline bg-[#faf8f4] px-4 py-3 sm:px-6">
          <div className="min-w-0">
            <p className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Biblioteca / Pessoa</p>
            <h2 className="truncate font-display text-2xl text-ink sm:text-3xl">{person?.displayName ?? 'Carregando…'}</h2>
          </div>
          <button ref={closeRef} onClick={close} className="flex min-h-11 min-w-11 items-center justify-center rounded-lg text-xl text-ink-soft hover:bg-surface-alt hover:text-brand" aria-label="Fechar">✕</button>
        </div>

        <div className="min-h-0 overflow-y-auto px-4 py-5 sm:px-6 sm:py-6">
          {isLoading && <p className="animate-pulse text-sm text-ink-faint">Carregando a pessoa…</p>}
          {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível abrir esta página.</LoadError>}
          {person && (
            <div className="flex flex-col gap-6">
              {person.aliases.length > 0 && (
                <p className="text-sm text-ink-soft">
                  Também aparece como <span className="text-ink">{person.aliases.join(' · ')}</span>
                </p>
              )}

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

                  {person.collections.length > 0 && (
                    <section aria-label="Coleções" className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-[11px] font-semibold uppercase tracking-widest text-ink-faint">Em coleções</span>
                      {person.collections.map((c) => (
                        <button
                          key={c.id}
                          onClick={() => toCollection(c.id)}
                          aria-label={`Abrir a coleção ${c.name}`}
                          className="min-h-10 rounded-full border border-border-hairline bg-white px-4 text-sm text-ink hover:bg-surface-alt"
                        >
                          {c.name}
                          <span className="ml-2 font-mono text-[11px] text-ink-faint">{c.works}</span>
                        </button>
                      ))}
                    </section>
                  )}

                  {works.isError ? (
                    <LoadError error={works.error} onRetry={works.refetch} retrying={works.isRefetching}>Não foi possível carregar as obras.</LoadError>
                  ) : (
                    <LibraryGrid
                      items={works.data?.data ?? []}
                      isLoading={works.isLoading}
                      isFetching={works.isFetching}
                      title={ROLE_NAMES[role] ? (role === 'author' ? 'Obras' : ROLE_NAMES[role].heading) : 'Obras'}
                      total={works.data?.total}
                    />
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
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
