import { useState } from 'react';
import { useAccounts, useBlockAccount, useUnblockAccount, useDeleteAccount, describeError } from '../api/admin';
import { formatDate } from '../format';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { Invitations } from './Invitations';
import { PasswordResets } from './PasswordResets';
import { TransferOwnership } from './TransferOwnership';

const ROLE = { owner: 'Dono', admin: 'Administrador', reader: 'Leitor' };

/**
 * The accounts. Whether the caller may block one comes from the server (`canBlock`),
 * so the rules about who may act on whom live in one place only.
 */
export function AccountsTab({ isOwner }) {
  const { data, isLoading, isError } = useAccounts();
  const block = useBlockAccount();
  const unblock = useUnblockAccount();
  const remove = useDeleteAccount();
  const [blocking, setBlocking] = useState(null);
  const [deleting, setDeleting] = useState(null);
  const accounts = data?.data || [];

  return (
    <div className="flex flex-col gap-5">
    <Section
      title="Contas"
      hint="Bloquear impede a pessoa de entrar e encerra na hora as sessões, os tokens de aplicativo e as conexões abertas dela. Notas e progresso ficam guardados, e o bloqueio se desfaz. Excluir é outra coisa: apaga a conta e os dados pessoais de vez."
    >
      {isLoading && <Loading />}
      {isError && <ErrorNote>Não foi possível carregar as contas.</ErrorNote>}
      {!isLoading && !isError && accounts.length === 0 && <Empty>Nenhuma conta.</Empty>}
      <ul className="divide-y divide-border-hairline">
        {accounts.map((account) => (
          <li key={account.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
            <div className="min-w-0">
              <p className="text-[14px] text-ink">
                {account.username}
                {account.isSelf && <span className="text-ink-faint"> (você)</span>}
                {account.blockedAt && (
                  <span className="ml-2 rounded bg-red-100 px-2 py-0.5 text-[11px] text-red-700">Bloqueada</span>
                )}
              </p>
              <p className="text-[12px] text-ink-faint">
                {ROLE[account.role] || account.role} · {account.email}
                {account.blockedAt && ` · bloqueada em ${formatDate(account.blockedAt)}`}
              </p>
            </div>
            <div className="flex gap-2">
              {account.canBlock && (
                account.blockedAt ? (
                  <Btn onClick={() => unblock.mutate(account.id)} disabled={unblock.isPending}>Desbloquear</Btn>
                ) : (
                  <Btn tone="danger" onClick={() => setBlocking(account)}>Bloquear</Btn>
                )
              )}
              {account.canRemove && <Btn tone="danger" onClick={() => setDeleting(account)}>Excluir</Btn>}
            </div>
          </li>
        ))}
      </ul>
      <ErrorNote>{block.isError ? describeError(block.error) : unblock.isError ? describeError(unblock.error) : remove.isError && describeError(remove.error)}</ErrorNote>

      {deleting && (
        <ConfirmDialog
          title={`Excluir ${deleting.username} de vez?`}
          message={
            <>
              <p>Apaga a conta e tudo o que é pessoal dela: notas, favoritos, progresso de leitura, sessões e tokens. Isso não pode ser desfeito.</p>
              <p className="mt-2">Os livros do acervo ficam. Para só impedir a entrada, use Bloquear, que se desfaz.</p>
            </>
          }
          requireText={deleting.username}
          choices={[{ label: 'Excluir conta', value: true, tone: 'danger' }]}
          onChoose={() => { remove.mutate({ id: deleting.id, username: deleting.username }); setDeleting(null); }}
          onCancel={() => setDeleting(null)}
        />
      )}

      {blocking && (
        <ConfirmDialog
          title={`Bloquear ${blocking.username}?`}
          message={<p>A pessoa sai de todos os aparelhos agora e não consegue entrar até você desbloquear. Nada dela é apagado.</p>}
          choices={[{ label: 'Bloquear', value: true, tone: 'danger' }]}
          onChoose={() => { block.mutate(blocking.id); setBlocking(null); }}
          onCancel={() => setBlocking(null)}
        />
      )}
    </Section>
    <PasswordResets />
    <Invitations isOwner={isOwner} />
    {isOwner && <TransferOwnership />}
    </div>
  );
}
