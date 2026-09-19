import { useState } from 'react';
import { useInvitations, useCreateInvitation, useRevokeInvitation, describeError } from '../api/admin';
import { formatDate } from '../format';
import { Btn, Empty, ErrorNote, Section } from './ui';

const STATE = { pending: 'Pendente', used: 'Usado', expired: 'Expirado', revoked: 'Revogado' };
const ROLE = { admin: 'Administrador', reader: 'Leitor' };

export const inviteLink = (token) => `${window.location.origin}/?invite=${encodeURIComponent(token)}`;

/**
 * Invitations: a single-use link, valid for seven days, that you hand to the
 * person yourself. The link is shown once, when it is created; the server keeps
 * only a hash, so it cannot be shown again.
 */
export function Invitations({ isOwner }) {
  const { data } = useInvitations();
  const create = useCreateInvitation();
  const revoke = useRevokeInvitation();
  const [role, setRole] = useState('reader');
  const [email, setEmail] = useState('');
  const [copied, setCopied] = useState(false);
  const created = create.data;
  const invitations = data?.data || [];

  const submit = (event) => {
    event.preventDefault();
    setCopied(false);
    create.mutate({ role, email: email.trim() }, { onSuccess: () => setEmail('') });
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(inviteLink(created.token));
      setCopied(true);
    } catch {
      setCopied(false); // the link is selectable in the box below
    }
  };

  return (
    <Section
      title="Convites"
      hint="Cada convite é um link de uso único, válido por 7 dias, que você entrega à pessoa. Se informar um e-mail, só ele pode usar o link. O Códice não envia nada."
    >
      <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
        {isOwner && (
          <label className="flex flex-col gap-1 text-[11px] uppercase tracking-wide text-ink-soft">
            Papel
            <select value={role} onChange={(event) => setRole(event.target.value)} className="rounded bg-surface px-2 py-2 text-[13px] normal-case text-ink">
              <option value="reader">Leitor</option>
              <option value="admin">Administrador</option>
            </select>
          </label>
        )}
        <label className="flex min-w-[220px] flex-1 flex-col gap-1 text-[11px] uppercase tracking-wide text-ink-soft">
          E-mail (opcional)
          <input
            type="email" value={email} onChange={(event) => setEmail(event.target.value)}
            placeholder="pessoa@exemplo.com"
            className="rounded bg-surface px-3 py-2 text-[13px] normal-case text-ink outline-none"
          />
        </label>
        <Btn tone="primary" type="submit" disabled={create.isPending}>Criar convite</Btn>
      </form>
      <ErrorNote>{create.isError && describeError(create.error)}</ErrorNote>

      {created && (
        <div role="status" className="mt-4 rounded-lg border border-brand/30 bg-brand/5 p-4">
          <p className="text-[13px] text-ink">
            Convite para {ROLE[created.role]}{created.email && ` (${created.email})`} criado. <strong>Copie o link agora:
            ele não aparece de novo.</strong>
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            <input readOnly value={inviteLink(created.token)} aria-label="Link do convite" onFocus={(event) => event.target.select()}
              className="min-w-[240px] flex-1 rounded bg-white px-3 py-2 font-mono text-[12px] text-ink" />
            <Btn onClick={copy}>{copied ? 'Copiado' : 'Copiar link'}</Btn>
          </div>
        </div>
      )}

      {invitations.length === 0 ? (
        <Empty>Nenhum convite emitido.</Empty>
      ) : (
        <ul className="mt-4 divide-y divide-border-hairline">
          {invitations.map((invitation) => (
            <li key={invitation.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <p className="text-[14px] text-ink">
                  {ROLE[invitation.role]}
                  {invitation.email && <span className="text-ink-soft"> · {invitation.email}</span>}
                  <span className="ml-2 rounded bg-surface-alt px-2 py-0.5 text-[11px] text-ink-soft">{STATE[invitation.state]}</span>
                </p>
                <p className="text-[12px] text-ink-faint">
                  {invitation.createdBy && `Emitido por ${invitation.createdBy} · `}
                  {invitation.state === 'used' ? `usado por ${invitation.usedBy}` : `vale até ${formatDate(invitation.expiresAt)}`}
                </p>
              </div>
              {invitation.canRevoke && (
                <Btn tone="danger" onClick={() => revoke.mutate(invitation.id)} disabled={revoke.isPending}>Revogar</Btn>
              )}
            </li>
          ))}
        </ul>
      )}
      <ErrorNote>{revoke.isError && describeError(revoke.error)}</ErrorNote>
    </Section>
  );
}
