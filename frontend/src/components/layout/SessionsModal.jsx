import { useEffect, useRef, useState } from 'react';
import { useSessions, useRevokeOtherSessions, useRevokeSession } from '../../features/auth/api/useSessions';
import { useAppTokens, useRevokeAppToken } from '../../features/auth/api/useAppTokens';
import { ConfirmDialog } from '../../features/admin/components/ConfirmDialog';
import { formatDate } from '../../features/admin/format';
import { LoadError } from '../ui/LoadError';
import { serverMessage } from '../../lib/serverMessage';
import { isTopmostDialog } from '../../lib/topDialog';
import { sessionLines } from './sessionText';

function Line({ session, onEnd }) {
  const lines = sessionLines(session);
  return (
    <li className="flex items-start justify-between gap-3 py-3">
      <div className="min-w-0">
        <p className="text-[14px] font-medium text-ink">
          {lines.title}
          <span className="ml-2 text-[12px] font-normal text-ink-faint">{lines.kind}</span>
          {session.current && <span className="ml-2 rounded bg-brand/10 px-2 py-0.5 text-[11px] font-medium text-brand">Esta sessão</span>}
        </p>
        <p className="text-[12px] text-ink-soft">{[lines.address, lines.entered].filter(Boolean).join(' · ')}</p>
        <p className={`text-[12px] ${lines.use === 'Ativa agora' ? 'text-success' : 'text-ink-faint'}`}>{lines.use}</p>
      </div>
      {!session.current && (
        <button
          onClick={() => onEnd(session)}
          aria-label={`Encerrar a sessão ${lines.title}`}
          className="shrink-0 rounded bg-surface-alt px-3 py-1.5 text-[12px] text-danger hover:brightness-95"
        >
          Encerrar
        </button>
      )}
    </li>
  );
}

/**
 * "Sessões e dispositivos" (UI-15, UI-21): where this account is signed in, and the apps it gave access to. A person
 * sees the device, the address the server saw and the last use of each session, and ends the ones that are not theirs;
 * ending is immediate (DEC-070). The session in use is marked and is not offered: signing out is the menu's.
 */
export function SessionsModal({ onClose, onOpenApps }) {
  const dialogRef = useRef(null);
  const { data, isLoading, isError, error, refetch, isFetching } = useSessions();
  const apps = useAppTokens();
  const end = useRevokeSession();
  const endOthers = useRevokeOtherSessions();
  const removeApp = useRevokeAppToken();
  const [ending, setEnding] = useState(null); // a session, or 'others'
  const [removing, setRemoving] = useState(null);

  useEffect(() => {
    const onKey = (event) => event.key === 'Escape' && isTopmostDialog(dialogRef.current) && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const sessions = data ?? [];
  const others = sessions.filter((s) => !s.current);
  const failure = end.isError
    ? serverMessage(end.error, 'Não foi possível encerrar a sessão.')
    : endOthers.isError ? serverMessage(endOthers.error, 'Não foi possível encerrar as sessões.') : '';

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label="Sessões e dispositivos" className="max-h-[92vh] w-full max-w-lg overflow-y-auto animate-pop-in rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Sessões e dispositivos</h2>
        <p className="mt-1 text-[13px] text-ink-soft">
          Os lugares onde a sua conta está aberta. Se algum não é seu, encerre: ele perde o acesso na hora e, para voltar, precisa da sua senha.
        </p>

        {isLoading && <p className="py-3 text-[13px] text-ink-faint">Carregando…</p>}
        {isError && <LoadError className="mt-3" error={error} onRetry={refetch} retrying={isFetching}>Não foi possível carregar as sessões.</LoadError>}
        {data && (
          <>
            <ul className="mt-3 divide-y divide-border-hairline">
              {sessions.map((session) => <Line key={session.id} session={session} onEnd={setEnding} />)}
            </ul>
            {failure && <p role="alert" className="mt-2 text-[13px] text-danger">{failure}</p>}
            {others.length > 0 && (
              <div className="mt-3">
                <button onClick={() => setEnding('others')} className="rounded bg-surface-alt px-3 py-1.5 text-[12px] text-danger hover:brightness-95">
                  Encerrar todas as outras
                </button>
              </div>
            )}
            {others.length === 0 && <p className="mt-3 text-[13px] text-ink-faint">Não há outra sessão aberta além desta.</p>}
          </>
        )}

        <h3 className="mt-6 text-[13px] font-medium text-ink">Aplicativos conectados</h3>
        <p className="mt-1 text-[12px] text-ink-faint">Os leitores (KOReader, Moon+ Reader…) que você liberou. Cada um tem uma senha só dele, que serve apenas para ler.</p>
        {apps.isLoading && <p className="py-2 text-[13px] text-ink-faint">Carregando…</p>}
        {apps.isError && <LoadError className="mt-2" error={apps.error} onRetry={apps.refetch} retrying={apps.isFetching}>Não foi possível carregar os aplicativos.</LoadError>}
        {apps.data && apps.data.length === 0 && <p className="py-2 text-[13px] text-ink-faint">Nenhum aplicativo conectado.</p>}
        <ul className="divide-y divide-border-hairline">
          {(apps.data || []).map((token) => (
            <li key={token.id} className="flex items-center justify-between gap-3 py-2">
              <div className="min-w-0">
                <p className="truncate text-[14px] font-medium text-ink">{token.name}</p>
                <p className="text-[12px] text-ink-faint">
                  Criado em {formatDate(token.createdAt)} · {token.lastUsedAt ? `usado em ${formatDate(token.lastUsedAt)}` : 'ainda não usado'}
                </p>
              </div>
              <button
                onClick={() => setRemoving(token)}
                aria-label={`Remover o acesso ${token.name}`}
                className="shrink-0 rounded bg-surface-alt px-3 py-1.5 text-[12px] text-danger hover:brightness-95"
              >
                Remover
              </button>
            </li>
          ))}
        </ul>
        {removeApp.isError && <p role="alert" className="mt-2 text-[13px] text-danger">Não foi possível remover o acesso.</p>}
        {onOpenApps && (
          <div className="mt-2">
            <button onClick={onOpenApps} className="rounded bg-surface-alt px-3 py-1.5 text-[12px] text-ink hover:brightness-95">Conectar um aplicativo</button>
          </div>
        )}

        <div className="mt-5 flex justify-end">
          <button onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Fechar</button>
        </div>
      </div>

      {ending && (
        <ConfirmDialog
          title={ending === 'others' ? 'Encerrar todas as outras sessões?' : `Encerrar a sessão ${sessionLines(ending).title}?`}
          message={ending === 'others'
            ? <p>Todos os outros aparelhos saem da sua conta na hora. Esta sessão continua. Cada um precisa entrar de novo, com a sua senha.</p>
            : <p>Esse aparelho sai da sua conta na hora e, para voltar, precisa da sua senha.</p>}
          choices={[{ label: ending === 'others' ? 'Encerrar todas' : 'Encerrar sessão', value: true, tone: 'danger' }]}
          onChoose={() => { if (ending === 'others') endOthers.mutate(); else end.mutate(ending.id); setEnding(null); }}
          onCancel={() => setEnding(null)}
        />
      )}
      {removing && (
        <ConfirmDialog
          title={`Remover o acesso “${removing.name}”?`}
          message={<p>O aplicativo perde o acesso na hora. Para voltar a usá-lo, crie outro acesso e preencha os dados de novo nele.</p>}
          choices={[{ label: 'Remover acesso', value: true, tone: 'danger' }]}
          onChoose={() => { removeApp.mutate(removing.id); setRemoving(null); }}
          onCancel={() => setRemoving(null)}
        />
      )}
    </div>
  );
}
