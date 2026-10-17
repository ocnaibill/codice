import { useEffect, useRef, useState } from 'react';
import { formatPairCode, isPairCode, useApprovePairing, useDenyPairing, useLookupPairing } from '../../features/auth/api/useDevicePairing';
import { ago } from '../../lib/ago';
import { serverMessage } from '../../lib/serverMessage';
import { useDialog } from '../../lib/useDialog';

const PLATFORMS = { kindle: 'Kindle', kobo: 'Kobo', android: 'Android' };

/**
 * Connects a reading device (a Kindle with KOReader) to this account by the code it shows (DEC-194). The person types the code, or comes
 * with it from the link or the QR of the device; sees which device asks, from where and when; and approves it or turns it down. Approving
 * is the only thing that gives the device an access, so the wording asks the person to approve only what is on their own screen now.
 */
export function PairReaderModal({ initialCode = '', onClose }) {
  const dialogRef = useRef(null);
  const [typed, setTyped] = useState(formatPairCode(initialCode));
  const [request, setRequest] = useState(null);
  const [name, setName] = useState('');
  const [done, setDone] = useState(null); // 'approved' | 'denied'
  const lookup = useLookupPairing();
  const approve = useApprovePairing();
  const deny = useDenyPairing();
  useDialog(dialogRef, { onEscape: onClose });

  const find = (code = typed) => {
    if (!isPairCode(code)) return;
    lookup.mutate(code.replace(/-/g, ''), {
      onSuccess: (data) => {
        setRequest(data);
        setName(data.name || '');
      },
    });
  };

  // Coming with a code (the link or the QR of the device), the request is already on screen.
  useEffect(() => {
    if (isPairCode(initialCode)) find(formatPairCode(initialCode));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const code = request ? request.userCode.replace(/-/g, '') : '';
  const label = request ? PLATFORMS[request.platform] || 'Leitor' : '';
  const when = request ? ago(request.requestedAt) : '';

  return (
    <div
      className="fixed inset-0 z-[60] flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label="Conectar um leitor" className="max-h-[92vh] w-full max-w-md overflow-y-auto animate-pop-in rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Conectar um leitor</h2>

        {done === 'approved' && (
          <>
            <p role="status" className="mt-3 text-[14px] text-ink">
              Pronto. O leitor termina de se conectar sozinho em alguns segundos. Ele aparece em Aplicativos como “{name || request?.name || label}”, e você pode removê-lo ali quando quiser.
            </p>
            <div className="mt-5 flex justify-end">
              <button onClick={onClose} className="rounded-lg bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110">Fechar</button>
            </div>
          </>
        )}

        {done === 'denied' && (
          <>
            <p role="status" className="mt-3 text-[14px] text-ink">Pedido negado. O leitor não recebeu acesso nenhum.</p>
            <div className="mt-5 flex justify-end">
              <button onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Fechar</button>
            </div>
          </>
        )}

        {!done && !request && (
          <form
            className="mt-3"
            onSubmit={(event) => {
              event.preventDefault();
              find();
            }}
          >
            <p className="text-[13px] text-ink-soft">
              No leitor, abra o plugin do Códice e escolha conectar: ele mostra um código de oito letras e números. Digite-o aqui.
            </p>
            <label className="mt-4 flex flex-col gap-1 text-[13px] font-medium text-ink">
              Código do leitor
              <input
                value={typed}
                onChange={(event) => setTyped(formatPairCode(event.target.value))}
                inputMode="text"
                autoCapitalize="characters"
                autoComplete="off"
                autoFocus
                placeholder="BCDF-2345"
                className="rounded-lg border border-border-hairline bg-surface px-3 py-2 text-center font-mono text-[20px] tracking-widest text-ink outline-none focus:border-brand"
              />
            </label>
            {lookup.isError && (
              <p role="alert" className="mt-2 text-[13px] text-danger">
                {serverMessage(lookup.error, 'Não foi possível procurar o pedido.')}
              </p>
            )}
            <div className="mt-5 flex justify-end gap-2">
              <button type="button" onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Cancelar</button>
              <button type="submit" disabled={!isPairCode(typed) || lookup.isPending} className="rounded-lg bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110 disabled:opacity-40">
                Procurar
              </button>
            </div>
          </form>
        )}

        {!done && request && (
          <div className="mt-3">
            <p className="text-[13px] text-ink-soft">Este leitor pediu para entrar na sua conta:</p>
            <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-[14px]">
              <dt className="text-ink-faint">Código</dt>
              <dd className="font-mono tracking-widest text-ink">{request.userCode}</dd>
              <dt className="text-ink-faint">Aparelho</dt>
              <dd className="text-ink">{label}{request.client ? ` · ${request.client}` : ''}</dd>
              <dt className="text-ink-faint">De onde</dt>
              <dd className="text-ink">{request.ip || 'endereço desconhecido'}</dd>
              <dt className="text-ink-faint">Pedido</dt>
              <dd className="text-ink">{when || 'agora há pouco'}</dd>
            </dl>
            <label className="mt-4 flex flex-col gap-1 text-[13px] font-medium text-ink">
              Nome deste leitor
              <input
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={60}
                placeholder={label}
                className="rounded-lg border border-border-hairline bg-surface px-3 py-2 text-[14px] font-normal text-ink outline-none focus:border-brand"
              />
            </label>
            <p className="mt-3 rounded-lg bg-surface-alt px-3 py-2 text-[12px] text-ink-soft">
              Aprove só se este mesmo código está, agora, na tela do seu leitor. Quem aprova dá ao aparelho acesso de leitura ao seu acervo; você
              pode tirar esse acesso quando quiser, em Aplicativos.
            </p>
            {(approve.isError || deny.isError) && (
              <p role="alert" className="mt-2 text-[13px] text-danger">
                {serverMessage(approve.error || deny.error, 'Não foi possível responder ao pedido.')}
              </p>
            )}
            <div className="mt-5 flex flex-wrap justify-end gap-2">
              <button
                onClick={() => deny.mutate(code, { onSuccess: () => setDone('denied') })}
                disabled={approve.isPending || deny.isPending}
                className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-danger hover:brightness-95 disabled:opacity-40"
              >
                Negar
              </button>
              <button
                onClick={() => approve.mutate({ code, name: name.trim() }, { onSuccess: () => setDone('approved') })}
                disabled={approve.isPending || deny.isPending}
                className="rounded-lg bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110 disabled:opacity-40"
              >
                Aprovar
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
