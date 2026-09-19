import { useState } from 'react';
import { usePasswordResets, useApproveReset, useRejectReset, describeError } from '../api/admin';
import { formatDate } from '../format';
import { ConfirmDialog } from './ConfirmDialog';
import { resetLink } from './Invitations';
import { Btn, Empty, ErrorNote, Section } from './ui';

const STATE = { requested: 'Aguardando', approved: 'Link entregue', rejected: 'Recusado', expired: 'Expirado', used: 'Usado' };
const ROLE = { admin: 'Administrador', reader: 'Leitor' };

/**
 * Requests from people who forgot their password. Approving makes a single-use
 * link, valid for one hour, that you hand over yourself once you are sure who is
 * asking; nothing is sent by the Códice. The link is shown once.
 */
export function PasswordResets() {
  const { data } = usePasswordResets();
  const approve = useApproveReset();
  const reject = useRejectReset();
  const [asking, setAsking] = useState(null);
  const [copied, setCopied] = useState(false);
  const requests = data?.data || [];
  const link = approve.data;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(resetLink(link.token));
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <Section
      title="Pedidos de redefinição de senha"
      hint="Quem esqueceu a senha faz um pedido na tela de login. Antes de aprovar, confirme por outro meio que é mesmo a pessoa; depois entregue o link a ela."
    >
      {link && (
        <div role="status" className="mb-4 rounded-lg border border-brand/30 bg-brand/5 p-4">
          <p className="text-[13px] text-ink">
            Link para <strong>{link.username}</strong>, válido por 1 hora e de uso único. <strong>Copie agora: ele não aparece de novo.</strong>
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            <input readOnly value={resetLink(link.token)} aria-label="Link de redefinição" onFocus={(event) => event.target.select()}
              className="min-w-[240px] flex-1 rounded bg-white px-3 py-2 font-mono text-[12px] text-ink" />
            <Btn onClick={copy}>{copied ? 'Copiado' : 'Copiar link'}</Btn>
          </div>
        </div>
      )}

      {requests.length === 0 ? (
        <Empty>Nenhum pedido.</Empty>
      ) : (
        <ul className="divide-y divide-border-hairline">
          {requests.map((request) => (
            <li key={request.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <p className="text-[14px] text-ink">
                  {request.username} <span className="text-ink-soft">· {ROLE[request.role] || request.role}</span>
                  <span className="ml-2 rounded bg-surface-alt px-2 py-0.5 text-[11px] text-ink-soft">{STATE[request.state]}</span>
                </p>
                <p className="text-[12px] text-ink-faint">
                  Pedido em {formatDate(request.requestedAt)}
                  {request.decidedBy && ` · decidido por ${request.decidedBy}`}
                  {request.state === 'approved' && request.linkExpiresAt && ` · link vale até ${formatDate(request.linkExpiresAt)}`}
                </p>
              </div>
              {request.canDecide && (
                <div className="flex gap-2">
                  <Btn tone="primary" onClick={() => setAsking(request)}>Aprovar</Btn>
                  <Btn onClick={() => reject.mutate(request.id)} disabled={reject.isPending}>Recusar</Btn>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      <ErrorNote>{approve.isError ? describeError(approve.error) : reject.isError && describeError(reject.error)}</ErrorNote>

      {asking && (
        <ConfirmDialog
          title={`Aprovar o pedido de ${asking.username}?`}
          message={
            <>
              <p>Isto gera um link para escolher uma nova senha. Quem tiver o link poderá entrar como {asking.username}, então só o entregue depois de ter certeza de quem pediu.</p>
              <p className="mt-2">Ao usar o link, a pessoa é desconectada de todos os aparelhos.</p>
            </>
          }
          choices={[{ label: 'Gerar link', value: true, tone: 'primary' }]}
          onChoose={() => { setCopied(false); approve.mutate(asking.id); setAsking(null); }}
          onCancel={() => setAsking(null)}
        />
      )}
    </Section>
  );
}
