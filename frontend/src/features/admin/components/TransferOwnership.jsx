import { useState } from 'react';
import { useAccounts, describeError } from '../api/admin';
import { useOwnershipTransfer, useStartTransfer, useCancelTransfer } from '../../ownership/api';
import { formatDate } from '../format';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, ErrorNote, Section } from './ui';

/**
 * Passing the ownership on, for the owner. It takes two steps: you offer it
 * (confirming with your password and saying what you become), and the other person
 * accepts by entering their password. Until then nothing changes, and you can
 * withdraw the offer.
 */
export function TransferOwnership() {
  const { data: accounts } = useAccounts();
  const { data: current } = useOwnershipTransfer();
  const start = useStartTransfer();
  const cancel = useCancelTransfer();
  const [targetId, setTargetId] = useState('');
  const [formerRole, setFormerRole] = useState(''); // no default: it is a choice you make
  const [password, setPassword] = useState('');
  const [confirming, setConfirming] = useState(false);
  const candidates = (accounts?.data || []).filter((account) => account.role !== 'owner' && !account.blockedAt);
  const target = candidates.find((account) => account.id === targetId);
  const pending = current?.transfer && current.outgoing ? current.transfer : null;
  const ready = targetId && formerRole && password;

  return (
    <Section
      title="Transferir a titularidade"
      hint="Passa o papel de dono para outra conta. Ela precisa aceitar informando a senha dela, e só então algo muda. Se você perdeu todo o acesso, isso não se faz aqui: existe um comando no servidor (veja a documentação)."
    >
      {pending ? (
        <div role="status" className="rounded-lg bg-surface p-4 text-[13px] text-ink">
          <p>
            Oferta feita a <strong>{pending.to}</strong>, aguardando a resposta até {formatDate(pending.expiresAt)}. Depois de aceita, você passa a ser {pending.formerRole === 'admin' ? 'administrador' : 'leitor'}.
          </p>
          <div className="mt-3"><Btn onClick={() => cancel.mutate()} disabled={cancel.isPending}>Cancelar a oferta</Btn></div>
        </div>
      ) : (
        <form className="flex flex-wrap items-end gap-3" onSubmit={(event) => { event.preventDefault(); if (ready) setConfirming(true); }}>
          <label className="flex flex-col gap-1 text-[11px] uppercase tracking-wide text-ink-soft">
            Nova dona ou novo dono
            <select value={targetId} onChange={(event) => setTargetId(event.target.value)} className="rounded bg-surface px-2 py-2 text-[13px] normal-case text-ink">
              <option value="">Escolha uma conta</option>
              {candidates.map((account) => <option key={account.id} value={account.id}>{account.username}</option>)}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[11px] uppercase tracking-wide text-ink-soft">
            Você passa a ser
            <select value={formerRole} onChange={(event) => setFormerRole(event.target.value)} className="rounded bg-surface px-2 py-2 text-[13px] normal-case text-ink">
              <option value="">Escolha</option>
              <option value="admin">Administrador</option>
              <option value="reader">Leitor</option>
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[11px] uppercase tracking-wide text-ink-soft">
            Sua senha
            <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password"
              className="rounded bg-surface px-3 py-2 text-[13px] normal-case text-ink outline-none" />
          </label>
          <Btn tone="danger" type="submit" disabled={!ready || start.isPending}>Oferecer a titularidade</Btn>
        </form>
      )}
      <ErrorNote>{start.isError ? describeError(start.error) : cancel.isError && describeError(cancel.error)}</ErrorNote>

      {confirming && target && (
        <ConfirmDialog
          title={`Oferecer a titularidade a ${target.username}?`}
          message={
            <>
              <p>Se {target.username} aceitar, ela ou ele passa a ser o dono deste acervo, e você passa a ser {formerRole === 'admin' ? 'administrador' : 'leitor'}. Você não poderá desfazer isso sozinho.</p>
              <p className="mt-2">Nada muda até a aceitação, e você pode cancelar a oferta antes.</p>
            </>
          }
          choices={[{ label: 'Oferecer', value: true, tone: 'danger' }]}
          onChoose={() => {
            setConfirming(false);
            start.mutate({ targetId, formerRole, password }, { onSuccess: () => { setPassword(''); setTargetId(''); setFormerRole(''); } });
          }}
          onCancel={() => setConfirming(false)}
        />
      )}
    </Section>
  );
}
