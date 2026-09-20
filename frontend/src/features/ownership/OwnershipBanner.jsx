import { useState } from 'react';
import { useOwnershipTransfer, useAcceptTransfer, useDeclineTransfer, useAckNotice } from './api';
import { describeError } from '../admin/api/admin';

const ROLE = { admin: 'administrador', reader: 'leitor' };

const NOTICE_TEXT = {
  owner_recovery_reset: () => 'Alguém com acesso ao servidor gerou um link de recuperação para a conta do dono. Todas as sessões do dono foram encerradas. Se não foi você, ou quem cuida do servidor, trate como um incidente.',
  instance_restored: (details) => `A instância foi restaurada de um backup${details?.backupCreatedAt ? ` feito em ${new Date(details.backupCreatedAt).toLocaleString('pt-BR')}` : ''}. Tudo o que aconteceu depois dele se perdeu, e sessões, tokens de aplicativo e convites precisam ser emitidos de novo. Confira se está tudo como esperado.`,
  owner_recovery_transfer: (details) => `A titularidade foi transferida por um comando no servidor${details?.from ? `: de ${details.from} para ${details.to}` : ''}. O antigo dono passou a ser ${ROLE[details?.formerRole] || 'leitor'}.`,
};

/**
 * What the signed-in person must see: notices about things done on the server, and
 * an offer of ownership waiting for their answer. Accepting asks for the password
 * again, so a session left open somewhere is not enough to take over the instance.
 */
export function OwnershipBanner({ me }) {
  const { data } = useOwnershipTransfer(!!me);
  const accept = useAcceptTransfer();
  const decline = useDeclineTransfer();
  const ack = useAckNotice();
  const [asking, setAsking] = useState(false);
  const [password, setPassword] = useState('');
  const offer = data?.transfer && !data.outgoing ? data.transfer : null;
  const notices = me?.notices || [];
  if (!offer && notices.length === 0) return null;

  return (
    <div className="flex flex-col gap-2 px-4 pt-3 sm:px-6">
      {notices.map((notice) => (
        <div key={notice.id} role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 p-3 text-[13px] text-red-800">
          <span>{(NOTICE_TEXT[notice.kind] || (() => 'Aviso de segurança.'))(notice.details)}</span>
          <button onClick={() => ack.mutate(notice.id)} className="rounded bg-white px-3 py-1 text-[12px] text-red-800 hover:brightness-95">Entendi</button>
        </div>
      ))}

      {offer && (
        <div role="status" className="rounded-lg border border-brand/30 bg-brand/5 p-3 text-[13px] text-ink">
          <p>
            <strong>{offer.from}</strong> quer transferir a titularidade deste acervo para você. Ao aceitar, você passa a ser o dono
            e {offer.from} passa a ser {ROLE[offer.formerRole]}. Nada muda até você aceitar.
          </p>
          {asking ? (
            <form className="mt-2 flex flex-wrap items-end gap-2" onSubmit={(event) => {
              event.preventDefault();
              accept.mutate(password, { onSuccess: () => { setAsking(false); setPassword(''); } });
            }}>
              <label className="flex flex-col gap-1 text-[11px] uppercase tracking-wide text-ink-soft">
                Sua senha, para confirmar
                <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoFocus required
                  autoComplete="current-password" className="rounded bg-white px-3 py-2 text-[13px] normal-case text-ink outline-none" />
              </label>
              <button type="submit" disabled={accept.isPending} className="rounded bg-brand px-3 py-2 text-[12px] text-white hover:brightness-110 disabled:opacity-50">Aceitar a titularidade</button>
              <button type="button" onClick={() => { setAsking(false); setPassword(''); }} className="px-2 py-2 text-[12px] text-ink-soft">Cancelar</button>
            </form>
          ) : (
            <div className="mt-2 flex gap-2">
              <button onClick={() => setAsking(true)} className="rounded bg-brand px-3 py-1.5 text-[12px] text-white hover:brightness-110">Aceitar…</button>
              <button onClick={() => decline.mutate()} disabled={decline.isPending} className="rounded bg-surface-alt px-3 py-1.5 text-[12px] text-ink hover:brightness-95">Recusar</button>
            </div>
          )}
          {accept.isError && <p role="alert" className="mt-2 text-red-700">{describeError(accept.error)}</p>}
          {decline.isError && <p role="alert" className="mt-2 text-red-700">{describeError(decline.error)}</p>}
        </div>
      )}
    </div>
  );
}
