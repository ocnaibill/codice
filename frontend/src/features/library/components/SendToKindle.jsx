import { KINDLE_FORMATS, sendFailure, useKindle, useSendToKindle } from '../../auth/api/useKindle';

/**
 * "Enviar para o Kindle" for one file of a work: shown when the server sends books by e-mail and the file is one a Kindle takes (EPUB,
 * PDF, TXT). Without an address it says where to put one, instead of failing.
 */
export function SendToKindle({ workId, file }) {
  const { data } = useKindle();
  const send = useSendToKindle();
  if (!data?.available || !KINDLE_FORMATS.includes(String(file.format || '').toLowerCase())) return null;
  const hasAddress = !!data.email;
  return (
    <>
      <button
        type="button"
        onClick={() => send.mutate({ workId, fileId: file.id })}
        disabled={send.isPending || !hasAddress}
        className="inline-flex min-h-10 items-center text-xs text-ink-soft hover:text-brand disabled:opacity-40"
        title={hasAddress ? `Manda este arquivo para ${data.email}` : 'Diga o e-mail do seu leitor em Preferências'}
      >
        {send.isPending ? 'Enviando…' : 'Enviar para o Kindle'}
      </button>
      {send.isSuccess && <span role="status" className="text-xs text-success">Enviado para {send.data.to}. Pode levar alguns minutos para chegar.</span>}
      {send.isError && <span role="alert" className="text-xs text-danger">{sendFailure(send.error)}</span>}
    </>
  );
}
