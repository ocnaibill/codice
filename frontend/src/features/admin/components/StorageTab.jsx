import { useState } from 'react';
import {
  useRoots, useAddRoot, useRemoveRoot, useScanRoot, useCleanups, useRetryCleanups, useOrphans, useTrashOrphans,
  useReorganizePreview, useReorganize, useBackup, describeError,
} from '../api/admin';
import { formatBytes, formatDate } from '../format';
import { ConfirmDialog } from './ConfirmDialog';
import { ImportFolder } from './ImportFolder';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';

function Roots({ isOwner }) {
  const { data, isLoading } = useRoots();
  const add = useAddRoot();
  const remove = useRemoveRoot();
  const scan = useScanRoot();
  const [path, setPath] = useState('');
  const [removing, setRemoving] = useState(null);
  const roots = data?.roots || [];

  return (
    <Section
      title="Pastas autorizadas"
      hint="Pastas do servidor que o acervo pode ler sem copiar. Só o dono autoriza uma pasta; varrer cataloga os livros novos."
    >
      {isLoading && <Loading />}
      {!isLoading && roots.length === 0 && <Empty>Nenhuma pasta autorizada.</Empty>}
      <ul className="divide-y divide-border-hairline">
        {roots.map((root) => (
          <li key={root.id} className="flex flex-wrap items-center justify-between gap-2 py-2">
            <code className="text-[13px] text-ink">{root.path}</code>
            <div className="flex gap-2">
              <Btn onClick={() => scan.mutate(root.id)} disabled={scan.isPending}>Varrer</Btn>
              {isOwner && <Btn tone="danger" onClick={() => setRemoving(root)}>Remover</Btn>}
            </div>
          </li>
        ))}
      </ul>
      {data?.managed && <p className="mt-3 text-[12px] text-ink-faint">Armazenamento gerenciado: {data.managed}</p>}
      {scan.isSuccess && <p role="status" className="mt-2 text-[13px] text-ink-soft">Varredura na fila; acompanhe em Trabalhos.</p>}
      {isOwner && (
        <form
          className="mt-4 flex flex-wrap gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (path.trim()) add.mutate(path.trim(), { onSuccess: () => setPath('') });
          }}
        >
          <input
            value={path}
            onChange={(event) => setPath(event.target.value)}
            placeholder="/caminho/absoluto/da/pasta"
            aria-label="Nova pasta autorizada"
            className="min-w-[240px] flex-1 rounded bg-surface px-3 py-2 text-[13px] outline-none"
          />
          <Btn tone="primary" type="submit" disabled={add.isPending || !path.trim()}>Autorizar pasta</Btn>
        </form>
      )}
      <ErrorNote>{add.isError ? describeError(add.error) : scan.isError ? describeError(scan.error) : remove.isError && describeError(remove.error)}</ErrorNote>

      {removing && (
        <ConfirmDialog
          title="Remover esta pasta?"
          message={<p>Os livros já catalogados dela continuam no acervo, mas ela deixa de ser varrida.</p>}
          choices={[{ label: 'Remover', value: true, tone: 'danger' }]}
          onChoose={() => { remove.mutate(removing.id); setRemoving(null); }}
          onCancel={() => setRemoving(null)}
        />
      )}
    </Section>
  );
}

function Reorganize() {
  const preview = useReorganizePreview();
  const apply = useReorganize();
  const [confirming, setConfirming] = useState(false);
  const plan = preview.data;

  return (
    <Section
      title="Reorganizar o acervo"
      hint="Coloca cada arquivo do armazenamento gerenciado na pasta Autor/Obra/… segundo os dados atuais. Veja o plano antes: nada muda até você confirmar."
      actions={<Btn onClick={() => { apply.reset(); preview.mutate(); }} disabled={preview.isPending}>Ver plano</Btn>}
    >
      {plan && plan.moves.length === 0 && <Empty>Nada a mover: {plan.unchanged} arquivo(s) já estão no lugar.</Empty>}
      {plan && plan.moves.length > 0 && (
        <>
          <p className="mb-2 text-[13px] text-ink-soft">{plan.moves.length} arquivo(s) seriam movidos; {plan.unchanged} já estão no lugar.</p>
          <ul className="max-h-72 divide-y divide-border-hairline overflow-y-auto text-[12px]">
            {plan.moves.map((move) => (
              <li key={move.fileId} className="py-2">
                <p className="text-ink">{move.title}</p>
                <p className="text-ink-faint">{move.from} → {move.to}</p>
              </li>
            ))}
          </ul>
          <div className="mt-3">
            <Btn tone="primary" onClick={() => setConfirming(true)} disabled={apply.isPending}>Mover {plan.moves.length} arquivo(s)</Btn>
          </div>
        </>
      )}
      {plan?.skipped?.length > 0 && <p className="mt-2 text-[12px] text-ink-faint">Ignorados: {plan.skipped.join('; ')}</p>}
      {apply.data && (
        <p role="status" className="mt-3 text-[13px] text-ink-soft">
          {apply.data.moved} arquivo(s) movido(s){apply.data.failures?.length > 0 && `, ${apply.data.failures.length} com falha`}.
        </p>
      )}
      <ErrorNote>{preview.isError ? describeError(preview.error) : apply.isError && (apply.error?.response?.status === 409
        ? 'O acervo mudou desde que o plano foi mostrado. Veja o plano de novo.' : describeError(apply.error))}</ErrorNote>

      {confirming && (
        <ConfirmDialog
          title="Reorganizar agora?"
          message={<p>Os arquivos listados serão movidos de pasta. Os livros continuam abrindo normalmente.</p>}
          choices={[{ label: 'Reorganizar', value: true, tone: 'primary' }]}
          onChoose={() => { setConfirming(false); apply.mutate(plan.hash); }}
          onCancel={() => setConfirming(false)}
        />
      )}
    </Section>
  );
}

function Cleanups() {
  const { data } = useCleanups();
  const retry = useRetryCleanups();
  const pending = data?.data || [];
  if (pending.length === 0) return null;
  return (
    <Section
      title="Originais que não puderam ser apagados"
      hint="Depois de mover ou importar, estes arquivos ficaram na origem. Tentamos de novo só se ainda forem exatamente o que foi copiado."
      actions={<Btn onClick={() => retry.mutate()} disabled={retry.isPending}>Tentar apagar de novo</Btn>}
    >
      <ul className="divide-y divide-border-hairline text-[13px]">
        {pending.map((item) => (
          <li key={item.id} className="py-2">
            <code className="text-ink">{item.path}</code>
            <p className="text-[12px] text-ink-faint">{item.reason}</p>
          </li>
        ))}
      </ul>
      {retry.data && <p role="status" className="mt-2 text-[13px] text-ink-soft">{retry.data.data?.removed ?? 0} removido(s).</p>}
    </Section>
  );
}

function Orphans() {
  const { data, isLoading } = useOrphans();
  const trash = useTrashOrphans();
  const [picked, setPicked] = useState({});
  const [confirming, setConfirming] = useState(false);
  const orphans = data?.data || [];
  const paths = orphans.filter((o) => picked[o.path]).map((o) => o.path);

  return (
    <Section
      title="Arquivos órfãos"
      hint="Arquivos no armazenamento gerenciado que o acervo não conhece (sobras de uma falha, por exemplo). Ir para a lixeira é recuperável."
      actions={<Btn tone="danger" disabled={paths.length === 0 || trash.isPending} onClick={() => setConfirming(true)}>Enviar {paths.length} para a lixeira</Btn>}
    >
      {isLoading && <Loading />}
      {!isLoading && orphans.length === 0 && <Empty>Nenhum arquivo órfão.</Empty>}
      <ul className="divide-y divide-border-hairline">
        {orphans.map((orphan) => (
          <li key={orphan.path} className="flex items-center gap-3 py-2 text-[13px]">
            <input
              type="checkbox"
              aria-label={`Selecionar ${orphan.path}`}
              checked={!!picked[orphan.path]}
              onChange={(event) => setPicked({ ...picked, [orphan.path]: event.target.checked })}
            />
            <code className="min-w-0 flex-1 truncate text-ink">{orphan.path}</code>
            <span className="text-ink-faint">{formatBytes(orphan.sizeBytes)}</span>
          </li>
        ))}
      </ul>
      <ErrorNote>{trash.isError && describeError(trash.error)}</ErrorNote>

      {confirming && (
        <ConfirmDialog
          title="Enviar para a lixeira?"
          message={<p>{paths.length} arquivo(s) vão para a lixeira. Dá para recuperá-los de lá até esvaziá-la.</p>}
          choices={[{ label: 'Enviar para a lixeira', value: true, tone: 'danger' }]}
          onChoose={() => { setConfirming(false); trash.mutate(paths, { onSuccess: () => setPicked({}) }); }}
          onCancel={() => setConfirming(false)}
        />
      )}
    </Section>
  );
}

const DAY = 24 * 60 * 60 * 1000;

/**
 * When the last backup was made here. Backups are made and restored with the
 * codice-admin command on the server, so this only tells whether one exists and
 * whether it is recent: the goal is a daily backup, so anything older than two days
 * is called out.
 */
function Backup() {
  const { data, isLoading } = useBackup();
  const last = data?.lastBackup;
  const stale = last && Date.now() - new Date(last.at).getTime() > 2 * DAY;
  return (
    <Section
      title="Backup"
      hint="O Códice não agenda backups: você agenda o comando no servidor. O pacote leva o banco e a lista de arquivos com seus hashes (e os arquivos, com --include-files). Sessões, tokens e convites nunca entram."
    >
      {isLoading && <Loading />}
      {!isLoading && !last && (
        <p role="alert" className="text-[13px] text-red-700">Nenhum backup registrado nesta instância.</p>
      )}
      {last && (
        <p className={`text-[13px] ${stale ? 'text-red-700' : 'text-ink'}`} role={stale ? 'alert' : undefined}>
          Último backup: {formatDate(last.at)} ({formatBytes(last.bytes)}
          {last.includesFiles ? `, com ${last.files} arquivo(s)` : ', só banco e lista de arquivos'}
          {last.encrypted ? ', criptografado' : ', sem criptografia'}).
          {stale && ' Faz mais de dois dias: a meta é um por dia.'}
        </p>
      )}
      <p className="mt-3 text-[12px] text-ink-faint">
        No servidor: <code>codice-admin backup --dir /backups --include-files</code>. Depois, <code>codice-admin verify-backup --deep</code> ensaia a restauração.
      </p>
    </Section>
  );
}

export function StorageTab({ isOwner }) {
  return (
    <div className="flex flex-col gap-5">
      <ImportFolder />
      <Roots isOwner={isOwner} />
      <Backup />
      <Reorganize />
      <Cleanups />
      <Orphans />
    </div>
  );
}
