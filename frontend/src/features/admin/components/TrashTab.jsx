import { useState } from 'react';
import {
  useTrash, useRestoreTrash, useDeleteTrash, useEmptyTrash, useSetTrashPolicy, usePreviewTrashPolicy,
  useApplyTrashPolicy, describeError,
} from '../api/admin';
import { formatBytes, formatDate } from '../format';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';

function Policy({ policy, isOwner }) {
  const [enabled, setEnabled] = useState(policy.enabled);
  const [days, setDays] = useState(policy.days);
  const save = useSetTrashPolicy();
  const preview = usePreviewTrashPolicy();
  const apply = useApplyTrashPolicy();
  const [confirming, setConfirming] = useState(false);

  if (!isOwner) {
    return (
      <p className="text-[13px] text-ink-soft">
        {policy.enabled ? `Itens são apagados de vez ${policy.days} dia(s) depois de irem para a lixeira.` : 'A lixeira não é esvaziada sozinha.'}
      </p>
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

export function TrashTab({ isOwner }) {
  const { data, isLoading, isError } = useTrash();
  const restore = useRestoreTrash();
  const remove = useDeleteTrash();
  const empty = useEmptyTrash();
  const [asking, setAsking] = useState(null); // { kind: 'delete', item } | { kind: 'empty' }
  const items = data?.items || [];

  return (
    <div className="flex flex-col gap-5">
      <Section
        title="Lixeira"
        hint="Arquivos de obras removidas do acervo. Ficam aqui até você apagar de vez; enquanto isso, dá para recuperar."
        actions={
          <Btn tone="danger" disabled={items.length === 0 || empty.isPending} onClick={() => setAsking({ kind: 'empty' })}>
            Esvaziar
          </Btn>
        }
      >
        {data && <p className="mb-3 text-[13px] text-ink-soft">Ocupa {formatBytes(data.totalBytes)} em {items.length} item(ns).</p>}
        {isLoading && <Loading />}
        {isError && <ErrorNote>Não foi possível carregar a lixeira.</ErrorNote>}
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
