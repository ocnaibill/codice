import { useEffect, useRef, useState } from 'react';
import { useRevokeAllUserSessions, useRevokeUserSession, useUserSessions } from '../../auth/api/useSessions';
import { sessionLines } from '../../../components/layout/sessionText';
import { LoadError } from '../../../components/ui/LoadError';
import { serverMessage } from '../../../lib/serverMessage';
import { isTopmostDialog } from '../../../lib/topDialog';
import { ConfirmDialog } from './ConfirmDialog';

/**
 * The sessions of another account, for who may cut its access (DEC-060): the device and the use of each, to end the
 * one that should not be there. The address is not in it: where a person signs in from is theirs. Ending is not
 * blocking: the person can sign in again with their password.
 */
export function UserSessionsModal({ account, onClose }) {
  const dialogRef = useRef(null);
  const { data, isLoading, isError, error, refetch, isFetching } = useUserSessions(account.id);
  const end = useRevokeUserSession(account.id);
  const endAll = useRevokeAllUserSessions(account.id);
  const [ending, setEnding] = useState(null); // a session, or 'all'

  useEffect(() => {
    const onKey = (event) => event.key === 'Escape' && isTopmostDialog(dialogRef.current) && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const failure = end.isError
    ? serverMessage(end.error, 'Não foi possível encerrar a sessão.')
    : endAll.isError ? serverMessage(endAll.error, 'Não foi possível encerrar as sessões.') : '';
  const sessions = data ?? [];

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label={`Sessões de ${account.username}`} className="max-h-[92vh] w-full max-w-lg overflow-y-auto animate-pop-in rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Sessões de {account.username}</h2>
        <p className="mt-1 text-[13px] text-ink-soft">
          Encerrar tira o aparelho da conta na hora, sem bloquear a pessoa: ela entra de novo com a senha dela. O endereço de onde ela entra não aparece aqui.
        </p>
        {isLoading && <p className="py-3 text-[13px] text-ink-faint">Carregando…</p>}
        {isError && <LoadError className="mt-3" error={error} onRetry={refetch} retrying={isFetching}>Não foi possível carregar as sessões.</LoadError>}
        {data && sessions.length === 0 && <p className="py-3 text-[13px] text-ink-faint">Nenhuma sessão aberta.</p>}
        <ul className="mt-2 divide-y divide-border-hairline">
          {sessions.map((session) => {
            const lines = sessionLines(session);
            return (
              <li key={session.id} className="flex items-start justify-between gap-3 py-3">
                <div className="min-w-0">
                  <p className="text-[14px] font-medium text-ink">{lines.title}<span className="ml-2 text-[12px] font-normal text-ink-faint">{lines.kind}</span></p>
                  <p className="text-[12px] text-ink-soft">{lines.entered}</p>
                  <p className={`text-[12px] ${lines.use === 'Ativa agora' ? 'text-success' : 'text-ink-faint'}`}>{lines.use}</p>
                </div>
                <button
                  onClick={() => setEnding(session)}
                  aria-label={`Encerrar a sessão ${lines.title}`}
                  className="shrink-0 rounded bg-surface-alt px-3 py-1.5 text-[12px] text-danger hover:brightness-95"
                >
                  Encerrar
                </button>
              </li>
            );
          })}
        </ul>
        {failure && <p role="alert" className="mt-2 text-[13px] text-danger">{failure}</p>}
        {sessions.length > 1 && (
          <div className="mt-3">
            <button onClick={() => setEnding('all')} className="rounded bg-surface-alt px-3 py-1.5 text-[12px] text-danger hover:brightness-95">Encerrar todas</button>
          </div>
        )}
        <div className="mt-5 flex justify-end">
          <button onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Fechar</button>
        </div>
      </div>

      {ending && (
        <ConfirmDialog
          title={ending === 'all' ? `Encerrar todas as sessões de ${account.username}?` : `Encerrar a sessão ${sessionLines(ending).title}?`}
          message={<p>{ending === 'all' ? 'Todos os aparelhos saem' : 'Esse aparelho sai'} da conta de {account.username} na hora. A conta não é bloqueada: a pessoa entra de novo com a senha dela.</p>}
          choices={[{ label: ending === 'all' ? 'Encerrar todas' : 'Encerrar sessão', value: true, tone: 'danger' }]}
          onChoose={() => { if (ending === 'all') endAll.mutate(); else end.mutate(ending.id); setEnding(null); }}
          onCancel={() => setEnding(null)}
        />
      )}
    </div>
  );
}
