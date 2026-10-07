import React from 'react';
import { EmptyState, Skeleton } from '../../../components/ui/EmptyState';
import { WorkCover } from '../../../components/ui/WorkCover';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { isStaff, useMe } from '../../auth/api/useMe';
import { formatCount } from '../../home/utils/format';
import { collectionReason, useCollections, useCreateCollection, useRestoreCollection } from '../api/useCollections';
import { collectionLine } from '../text';

function NewCollectionForm({ onDone }) {
  const [name, setName] = React.useState('');
  const [message, setMessage] = React.useState('');
  const open = useGlobalStore((state) => state.openCollection);
  const create = useCreateCollection();
  const submit = (event) => {
    event.preventDefault();
    if (!name.trim()) return;
    setMessage('');
    create.mutate(name, {
      onSuccess: (made) => {
        onDone();
        open(made.id); // empty: the next thing is putting works in it
      },
      onError: (error) => setMessage(collectionReason(error, 'Não foi possível criar a coleção.')),
    });
  };
  return (
    <form onSubmit={submit} className="mb-4 flex flex-wrap items-end gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <label className="flex min-w-[220px] flex-1 flex-col gap-1 text-sm text-ink-soft">
        Nome da coleção
        <input
          autoFocus
          value={name}
          maxLength={512}
          onChange={(event) => setName(event.target.value)}
          className="min-h-11 rounded-lg border border-border-hairline bg-surface px-3 text-ink"
        />
      </label>
      <button type="submit" disabled={!name.trim() || create.isPending} className="min-h-11 rounded-lg bg-brand px-4 text-sm font-semibold text-white hover:bg-brand-light disabled:opacity-40">
        Criar
      </button>
      <button type="button" onClick={onDone} className="min-h-11 rounded-lg border border-border-hairline px-4 text-sm text-ink hover:bg-surface-alt">
        Cancelar
      </button>
      {message && <p role="alert" className="basis-full text-sm text-danger">{message}</p>}
    </form>
  );
}

function CollectionCard({ item, onOpen, canRestore }) {
  const restore = useRestoreCollection();
  const [message, setMessage] = React.useState('');
  return (
    <article className="library-book">
      <div className="library-book-body">
        <button onClick={() => onOpen(item.id)} title="Ver as obras da coleção" aria-label={`Abrir a coleção ${item.name}`} className="library-book-cover">
          <WorkCover item={{ coverUrl: item.coverUrl, title: item.name, author: '' }} />
        </button>
        <div className="library-book-meta">
          <p className="library-book-status">{item.retired ? 'Coleção aposentada' : item.kind === 'personal' ? 'Coleção pessoal' : 'Coleção'}</p>
          <h3 title={item.name}>{item.name}</h3>
          <p className="library-author">{collectionLine(item)}</p>
        </div>
      </div>
      {canRestore && item.retired && (
        <div className="library-book-actions">
          <button
            onClick={() => restore.mutate(item.id, { onError: (error) => setMessage(collectionReason(error, 'Não foi possível restaurar.')) })}
            disabled={restore.isPending}
            className="text-xs font-semibold text-brand"
            aria-label={`Restaurar a coleção ${item.name}`}
          >
            Restaurar
          </button>
          {message && <span role="alert" className="text-xs text-danger">{message}</span>}
        </div>
      )}
    </article>
  );
}

/**
 * The collections of the library (#184, DEC-130), as cards: what the series of the works made, and the ones owner and admin
 * made by hand. Owner and admin also make a new one here and see the retired ones, to restore them.
 */
export function CollectionsGrid({ viewMode = 'grid' }) {
  const staff = isStaff(useMe().data);
  const open = useGlobalStore((state) => state.openCollection);
  const [page, setPage] = React.useState(1);
  const [retired, setRetired] = React.useState(false);
  const [creating, setCreating] = React.useState(false);
  const { data, isLoading, isFetching, isError, refetch } = useCollections({ page, retired: staff && retired });
  const items = data?.data ?? [];
  const totalPages = data?.totalPages ?? 1;

  return (
    <section aria-busy={!!(isLoading || isFetching)} aria-label="Coleções">
      <div className="library-section-heading">
        <h2>{retired ? 'Coleções aposentadas' : 'Coleções'}</h2>
        <div className="flex flex-wrap items-center gap-2">
          {data?.total != null && <span className="library-eyebrow">[ {formatCount(data.total)} {data.total === 1 ? 'coleção' : 'coleções'} ]</span>}
          {staff && (
            <>
              <button
                onClick={() => { setRetired((v) => !v); setPage(1); setCreating(false); }}
                aria-pressed={retired}
                className="min-h-9 rounded-lg border border-border-hairline px-3 text-xs text-ink hover:bg-surface-alt aria-pressed:bg-surface-alt"
              >
                Aposentadas
              </button>
              {!retired && (
                <button onClick={() => setCreating(true)} className="min-h-9 rounded-lg bg-brand px-3 text-xs font-semibold text-white hover:bg-brand-light">
                  Nova coleção
                </button>
              )}
            </>
          )}
        </div>
      </div>
      {creating && !retired && <NewCollectionForm onDone={() => setCreating(false)} />}
      {isError ? (
        <div role="alert" className="library-error">
          <p>Não foi possível carregar as coleções.</p>
          <button className="library-button" onClick={() => refetch()}>Tentar novamente</button>
        </div>
      ) : isLoading ? (
        <div className="library-books" data-view={viewMode}>
          {Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className={viewMode === 'list' ? 'h-28 w-full' : 'h-[300px] w-full'} />)}
        </div>
      ) : items.length === 0 ? (
        <EmptyState>
          {retired
            ? 'Nenhuma coleção aposentada.'
            : staff
              ? 'Ainda não há coleções. Elas nascem do nome da série no metadado das obras, ou você cria uma e acrescenta as obras.'
              : 'Ainda não há coleções. Elas aparecem quando as obras de uma série entram no acervo.'}
        </EmptyState>
      ) : (
        <div className="library-books" data-view={viewMode}>
          {items.map((item) => <CollectionCard key={item.id} item={item} onOpen={open} canRestore={staff} />)}
        </div>
      )}
      {totalPages > 1 && (
        <nav className="library-pagination" aria-label="Páginas das coleções">
          <button className="library-button" disabled={page <= 1 || isFetching} onClick={() => setPage(page - 1)}>Anterior</button>
          <span aria-live="polite">{page} de {totalPages}</span>
          <button className="library-button" disabled={page >= totalPages || isFetching} onClick={() => setPage(page + 1)}>Próxima</button>
        </nav>
      )}
    </section>
  );
}
