import { useEffect, useState } from 'react';
import {
  useTrash, useRestoreTrash, useDeleteTrash, useEmptyTrash, useSetTrashPolicy, usePreviewTrashPolicy,
  useApplyTrashPolicy, useRetiredWorks, useRestoreWork, usePurgeWork, describeError,
} from '../api/admin';
import { formatBytes, formatDate } from '../format';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';
import { PermissionNote } from '../../../components/ui/PermissionNote';

function Policy({ policy, isOwner }) {
  const [enabled, setEnabled] = useState(policy.enabled);
  const [days, setDays] = useState(policy.days);
  const save = useSetTrashPolicy();
  const preview = usePreviewTrashPolicy();
  const apply = useApplyTrashPolicy();
  const [confirming, setConfirming] = useState(false);

  if (!isOwner) {
    return (
      <div className="flex flex-col gap-2">
        <p className="text-[13px] text-ink-soft">
          {policy.enabled ? `Itens são apagados de vez ${policy.days} dia(s) depois de irem para a lixeira.` : 'A lixeira não é esvaziada sozinha.'}
        </p>
        <PermissionNote>Só o dono do acervo muda isso e apaga de vez.</PermissionNote>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <label className="flex items-center gap-2 text-[13px] text-ink">
        <input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
        Apagar itens da lixeira sozinho depois de
        <input
          type="number" min="1" max="3650" value={days}
          aria-label="Dias na lixeira"
          onChange={(event) => setDays(Number(event.target.value))}
          className="w-20 rounded bg-surface px-2 py-1 text-[13px]"
        />
        dia(s)
      </label>
      <div className="flex flex-wrap gap-2">
        <Btn onClick={() => save.mutate({ enabled, days })} disabled={save.isPending}>Salvar regra</Btn>
        <Btn onClick={() => preview.mutate(days)} disabled={preview.isPending}>Ver o efeito nos itens atuais</Btn>
      </div>
      {preview.data && (
        <p role="status" className="text-[13px] text-ink-soft">
          Aplicar {preview.data.days} dia(s) aos {preview.data.items} item(ns) que já estão na lixeira apagaria de vez{' '}
          {preview.data.alreadyDue} deles na próxima limpeza.{' '}
          <button className="text-brand underline" onClick={() => setConfirming(true)}>Aplicar aos itens atuais</button>
        </p>
      )}
      {save.isSuccess && <p role="status" className="text-[13px] text-ink-soft">Regra salva.</p>}
      <ErrorNote>{save.isError ? describeError(save.error) : apply.isError && describeError(apply.error)}</ErrorNote>

      {confirming && (
        <ConfirmDialog
          title="Aplicar aos itens atuais?"
          message={<p>Os itens que já passaram do prazo serão apagados de vez na próxima limpeza.</p>}
          choices={[{ label: 'Aplicar', value: true, tone: 'danger' }]}
          onChoose={() => { setConfirming(false); apply.mutate(days); }}
          onCancel={() => setConfirming(false)}
        />
      )}
    </div>
  );
}

/** What a retired work keeps, in one line: the formats, how many files and how much of the disk they take, or that they are in the trash. */
function keepsLine(work) {
  if (work.files === 0) return 'Sem arquivo guardado.';
  const formats = work.formats.length > 0 ? `${work.formats.join(' + ')} · ` : '';
  const count = `${work.files} arquivo${work.files === 1 ? '' : 's'}`;
  if (work.inTrash === work.files) return `${formats}${count}, na lixeira dos arquivos, logo abaixo.`;
  if (work.inTrash > 0) return `${formats}${count} (${work.inTrash} na lixeira dos arquivos, logo abaixo) · ${formatBytes(work.sizeBytes)} no disco.`;
  return `${formats}${count} · ${formatBytes(work.sizeBytes)} no disco.`;
}

/**
 * The works that were retired from the catalog: the readers do not see them, and their files, notes and progress are kept. They come back
 * with "Restaurar"; "Apagar de vez" sends their files to the trash below, where they can still be recovered, and the work is gone for good
 * only when those files are deleted from the disk. A work whose files are in the trash is restored by recovering them first.
 */
function RetiredWorks() {
  const [page, setPage] = useState(1);
  const { data, isLoading, isError, error, refetch, isRefetching } = useRetiredWorks(page);
  const restore = useRestoreWork();
  const purge = usePurgeWork();
  const [asking, setAsking] = useState(null); // the work waiting for a yes to "delete for good"
  const [done, setDone] = useState('');
  const works = data?.data ?? [];
  const busy = restore.isPending || purge.isPending;
  const totalPages = data?.totalPages ?? 1;
  // The last page has none left (its works were restored or deleted): back to the last one there is.
  const gone = !!data && works.length === 0 && page > 1;
  useEffect(() => {
    if (gone) setPage(Math.max(1, totalPages));
  }, [gone, totalPages]);

  const doRestore = (work) => {
    setDone('');
    restore.mutate(work.id, { onSuccess: () => setDone(`“${work.title}” voltou ao acervo.`) });
  };
  const doPurge = (work) => {
    setAsking(null);
    setDone('');
    purge.mutate(work.id, {
      onSuccess: (result) =>
        setDone(
          result?.deleted
            ? `“${work.title}” foi apagada de vez.`
            : `${result?.trashed === 1 ? 'O arquivo' : 'Os arquivos'} de “${work.title}” ${result?.trashed === 1 ? 'foi' : 'foram'} para a lixeira, logo abaixo. A obra sai desta lista quando ${result?.trashed === 1 ? 'ele for apagado' : 'eles forem apagados'} do disco.`
        ),
    });
  };

  return (
    <Section
      title="Obras retiradas"
      hint="Obras que saíram do acervo: os leitores não as veem, mas os arquivos, as notas e o progresso continuam guardados. Restaure para trazê-las de volta, ou apague de vez para mandar os arquivos à lixeira, logo abaixo."
    >
      {data && <p className="mb-3 text-[13px] text-ink-soft">{data.total === 0 ? 'Nenhuma obra retirada.' : `${data.total} obra${data.total === 1 ? '' : 's'} retirada${data.total === 1 ? '' : 's'}.`}</p>}
      {isLoading && <Loading />}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as obras retiradas.</LoadError>}
      <ul className="divide-y divide-border-hairline" aria-label="Obras retiradas">
        {works.map((work) => {
          const allInTrash = work.files > 0 && work.inTrash === work.files;
          const canPurge = work.files === 0 || work.inTrash < work.files;
          return (
            <li key={work.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <p className="text-[14px] text-ink">{work.title}</p>
                <p className="text-[12px] text-ink-faint">
                  {[work.author, `retirada em ${formatDate(work.retiredAt)}${work.retiredBy ? ` por ${work.retiredBy}` : ''}`].filter(Boolean).join(' · ')}
                </p>
                <p className="text-[12px] text-ink-faint">{keepsLine(work)}</p>
              </div>
              <div className="flex gap-2">
                <Btn
                  onClick={() => doRestore(work)}
                  disabled={busy || work.inTrash > 0}
                  title={work.inTrash > 0 ? 'Recupere os arquivos na lixeira, logo abaixo, para restaurar a obra' : undefined}
                  aria-label={`Restaurar “${work.title}”`}
                >
                  Restaurar
                </Btn>
                {canPurge && (
                  <Btn tone="danger" onClick={() => setAsking(work)} disabled={busy} aria-label={`Apagar de vez “${work.title}”`}>
                    Apagar de vez
                  </Btn>
                )}
                {allInTrash && <span className="self-center text-[12px] text-ink-faint">Falta apagar os arquivos, abaixo.</span>}
              </div>
            </li>
          );
        })}
      </ul>
      {totalPages > 1 && (
        <nav className="mt-3 flex items-center gap-3 text-[12px] text-ink-soft" aria-label="Páginas das obras retiradas">
          <Btn onClick={() => setPage(page - 1)} disabled={page <= 1}>Anterior</Btn>
          <span>Página {page} de {totalPages}</span>
          <Btn onClick={() => setPage(page + 1)} disabled={page >= totalPages}>Próxima</Btn>
        </nav>
      )}
      {done && <p role="status" className="mt-3 text-[13px] text-ink-soft">{done}</p>}
      <ErrorNote>{restore.isError ? describeError(restore.error) : purge.isError && describeError(purge.error)}</ErrorNote>

      {asking && (
        <ConfirmDialog
          title="Apagar esta obra de vez?"
          message={
            asking.files === 0 ? (
              <p>“{asking.title}” não tem arquivo guardado aqui: a obra será apagada de vez. As anotações continuam, sem a obra.</p>
            ) : (
              <p>
                {asking.files === 1 ? 'O arquivo' : `Os ${asking.files} arquivos`} de “{asking.title}” ({formatBytes(asking.sizeBytes)}) {asking.files === 1 ? 'vai' : 'vão'} para a lixeira, logo abaixo,
                de onde ainda dá para {asking.files === 1 ? 'recuperá-lo' : 'recuperá-los'}. A obra sai desta lista quando {asking.files === 1 ? 'ele for apagado' : 'eles forem apagados'} do disco. As anotações
                continuam, sem a obra.
              </p>
            )
          }
          choices={[{ label: 'Apagar de vez', value: true, tone: 'danger' }]}
          onChoose={() => doPurge(asking)}
          onCancel={() => setAsking(null)}
        />
      )}
    </Section>
  );
}

export function TrashTab({ isOwner }) {
  const { data, isLoading, isError, error, refetch, isRefetching } = useTrash();
  const restore = useRestoreTrash();
  const remove = useDeleteTrash();
  const empty = useEmptyTrash();
  const [asking, setAsking] = useState(null); // { kind: 'delete', item } | { kind: 'empty' }
  const items = data?.items || [];

  return (
    <div className="flex flex-col gap-5">
      <RetiredWorks />
      <Section
        title="Arquivos na lixeira"
        hint="Arquivos de obras que você apagou de vez. Ficam aqui até você apagá-los do disco; enquanto isso, dá para recuperar."
        actions={
          <Btn tone="danger" disabled={items.length === 0 || empty.isPending} onClick={() => setAsking({ kind: 'empty' })}>
            Esvaziar
          </Btn>
        }
      >
        {data && <p className="mb-3 text-[13px] text-ink-soft">Ocupa {formatBytes(data.totalBytes)} em {items.length} item(ns).</p>}
        {isLoading && <Loading />}
        {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar a lixeira.</LoadError>}
        {!isLoading && !isError && items.length === 0 && <Empty>A lixeira está vazia.</Empty>}
        <ul className="divide-y divide-border-hairline">
          {items.map((item) => (
            <li key={item.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <p className="text-[14px] text-ink">{item.workTitle || item.originalPath}</p>
                <p className="text-[12px] text-ink-faint">
                  {item.originalPath} · {formatBytes(item.sizeBytes)} · na lixeira desde {formatDate(item.trashedAt)}
                  {item.purgeAfter && ` · apagado de vez em ${formatDate(item.purgeAfter)}`}
                </p>
              </div>
              <div className="flex gap-2">
                <Btn onClick={() => restore.mutate(item.id)} disabled={restore.isPending}>Recuperar</Btn>
                <Btn tone="danger" onClick={() => setAsking({ kind: 'delete', item })}>Apagar de vez</Btn>
              </div>
            </li>
          ))}
        </ul>
        <ErrorNote>{restore.isError ? describeError(restore.error) : remove.isError ? describeError(remove.error) : empty.isError && describeError(empty.error)}</ErrorNote>
      </Section>

      {data?.policy && (
        <Section title="Limpeza automática" hint="Por padrão a lixeira nunca se esvazia sozinha.">
          <Policy key={`${data.policy.enabled}-${data.policy.days}`} policy={data.policy} isOwner={isOwner} />
        </Section>
      )}

      {asking?.kind === 'delete' && (
        <ConfirmDialog
          title="Apagar de vez?"
          message={<p>“{asking.item.workTitle || asking.item.originalPath}” ({formatBytes(asking.item.sizeBytes)}) será apagado do disco. Isso não pode ser desfeito.</p>}
          choices={[{ label: 'Apagar de vez', value: true, tone: 'danger' }]}
          onChoose={() => { remove.mutate(asking.item.id); setAsking(null); }}
          onCancel={() => setAsking(null)}
        />
      )}
      {asking?.kind === 'empty' && (
        <ConfirmDialog
          title="Esvaziar a lixeira?"
          message={<p>Os {items.length} item(ns), {formatBytes(data?.totalBytes)} no total, serão apagados do disco. Isso não pode ser desfeito.</p>}
          choices={[{ label: 'Esvaziar', value: true, tone: 'danger' }]}
          onChoose={() => { empty.mutate(); setAsking(null); }}
          onCancel={() => setAsking(null)}
        />
      )}
    </div>
  );
}
