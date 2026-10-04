import { useState } from 'react';
import { useDataExport, useDeleteDataExport, useRequestDataExport } from '../api/useDataExport';
import { downloadFile } from '../../../lib/download';
import { formatBytes, formatDate, formatLeft } from '../../admin/format';
import { LoadError } from '../../../components/ui/LoadError';
import { Notice } from '../../../components/ui/Notice';
import { serverMessage } from '../../../lib/serverMessage';

/**
 * "Meus dados": the file of everything that is the person's own, made by the server when asked, kept for a day and
 * taken only by them. It says what is in it and what is not (no book, no password), and what happens to it.
 */
export function MyData() {
  const { data, isLoading, isError, error, refetch, isFetching } = useDataExport();
  const request = useRequestDataExport();
  const remove = useDeleteDataExport();
  const [saving, setSaving] = useState(false);
  const [saveFailed, setSaveFailed] = useState('');
  const item = data?.export ?? null;

  const take = async () => {
    setSaving(true);
    setSaveFailed('');
    try {
      await downloadFile(`/auth/export/${item.id}/download`, 'codice-meus-dados.zip');
    } catch (err) {
      setSaveFailed(err?.response?.status === 410 || err?.response?.status === 404
        ? 'O arquivo já expirou. Prepare outro.'
        : 'Não foi possível baixar o arquivo. Tente de novo.');
      refetch();
    } finally {
      setSaving(false);
    }
  };

  const failure = request.isError && !(request.error?.response?.status === 409)
    ? serverMessage(request.error, 'Não foi possível pedir o arquivo.')
    : '';

  return (
    <section aria-labelledby="my-data-title" className="mt-6 border-t border-border-hairline pt-5">
      <h3 id="my-data-title" className="text-[13px] font-medium text-ink">Meus dados</h3>
      <p className="mt-1 text-[12px] text-ink-soft">
        Um arquivo ZIP com tudo o que é seu: anotações e destaques (em Markdown e em JSON), favoritos, o seu progresso de leitura, conceitos
        e relações, tags, as preferências, onde a sua conta está aberta, os aplicativos que você liberou e o registro das suas entradas.
        <strong className="font-medium text-ink"> Não tem</strong> os livros (são do acervo), a sua senha nem segredo nenhum.
      </p>

      {isLoading && <p className="py-2 text-[13px] text-ink-faint">Carregando…</p>}
      {isError && <LoadError className="mt-2" error={error} onRetry={refetch} retrying={isFetching}>Não foi possível ver o estado do seu arquivo.</LoadError>}

      {data && (
        <div className="mt-3 flex flex-col gap-3">
          {item?.state === 'pending' && (
            <Notice tone="info" title="Preparando o seu arquivo">Pode levar um minuto. Você pode fechar esta janela: ele fica pronto de qualquer jeito.</Notice>
          )}
          {item?.state === 'ready' && (
            <Notice tone="success" title="Seu arquivo está pronto">
              {formatBytes(item.bytes)}. Fica disponível por mais {formatLeft(new Date(item.expiresAt).getTime() - Date.now())} (até {formatDate(item.expiresAt)}),
              e só você consegue baixá-lo.
            </Notice>
          )}
          {item?.state === 'expired' && <Notice tone="warning" title="O arquivo anterior expirou">Ele some do servidor 24 horas depois de pronto. Prepare outro se precisar.</Notice>}
          {item?.state === 'failed' && <Notice tone="danger" title="Não foi possível preparar o arquivo">Tente de novo. Se continuar falhando, avise quem cuida do servidor.</Notice>}
          {request.isError && request.error?.response?.status === 409 && (
            <p role="status" className="text-[13px] text-ink-soft">Já estamos preparando um arquivo para você.</p>
          )}
          {failure && <p role="alert" className="text-[13px] text-danger">{failure}</p>}
          {saveFailed && <p role="alert" className="text-[13px] text-danger">{saveFailed}</p>}

          <div className="flex flex-wrap gap-2">
            {item?.state === 'ready' && (
              <button onClick={take} disabled={saving} className="rounded bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110 disabled:opacity-50">
                {saving ? 'Baixando…' : 'Baixar o arquivo'}
              </button>
            )}
            {item?.state !== 'pending' && (
              <button
                onClick={() => { setSaveFailed(''); request.mutate(); }}
                disabled={request.isPending}
                className={`rounded px-4 py-2 text-[13px] font-medium disabled:opacity-50 ${item?.state === 'ready' ? 'bg-surface-alt text-ink hover:brightness-95' : 'bg-brand text-white hover:brightness-110'}`}
              >
                {request.isPending ? 'Pedindo…' : item?.state === 'ready' ? 'Preparar outro' : 'Preparar o meu arquivo'}
              </button>
            )}
            {item?.state === 'ready' && (
              <button onClick={() => remove.mutate(item.id)} disabled={remove.isPending} className="rounded bg-surface-alt px-4 py-2 text-[13px] text-danger hover:brightness-95 disabled:opacity-50">
                Apagar o arquivo agora
              </button>
            )}
          </div>
          {remove.isError && <p role="alert" className="text-[13px] text-danger">Não foi possível apagar o arquivo.</p>}
          <p className="text-[12px] text-ink-faint">Cada pedido substitui o anterior, e cada pessoa pode pedir até 3 por hora.</p>
        </div>
      )}
    </section>
  );
}
