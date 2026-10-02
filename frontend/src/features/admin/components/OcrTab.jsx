import { useOcr, useOcrSettings, useRetryOcr, useSetOcr, describeError } from '../api/admin';
import { fileProgress, languageName } from '../../../lib/ocr';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';

const ENGINE_STATE = {
  idle: 'Parado, esperando páginas para ler.',
  working: 'Lendo páginas agora.',
  error: 'O serviço de OCR não conseguiu trabalhar.',
};

const TONE = { ok: 'text-success', warn: 'text-amber-800', plain: 'text-ink-faint' };

function Settings({ isOwner }) {
  const { data: state, isLoading, isError } = useOcrSettings();
  const save = useSetOcr();
  if (isLoading) return <Loading />;
  if (isError || !state) return <ErrorNote>Não foi possível carregar a configuração do OCR.</ErrorNote>;

  const chosen = String(state.language || '').split('+').filter(Boolean);
  const change = (enabled, language = state.language) => save.mutate({ enabled, language });
  // Choosing a language keeps the order of the ones already chosen, and the last one cannot be taken away.
  const toggle = (code) => {
    const next = chosen.includes(code) ? chosen.filter((c) => c !== code) : [...chosen, code];
    if (next.length > 0) change(state.enabled, next.join('+'));
  };

  return (
    <Section
      title="Leitura de páginas escaneadas (OCR)"
      hint="Páginas de PDF que são só imagem não podem ser buscadas. O OCR lê essas imagens uma vez, em segundo plano, e guarda o texto para todos. O arquivo original não muda e o leitor continua mostrando a imagem. O texto reconhecido pode ter erros."
    >
      {!state.available ? (
        <ErrorNote>O serviço de OCR não está em execução. Atualize ou reinicie a pilha completa do Códice (o serviço ocr).</ErrorNote>
      ) : (
        <p className="text-xs text-ink-soft">
          Motor: {state.engine} {state.engineVersion}. {ENGINE_STATE[state.state] || ''}
        </p>
      )}
      {state.error && state.available && <ErrorNote>{state.error}</ErrorNote>}

      {isOwner ? (
        <>
          <label className="mt-4 flex items-start gap-3 text-sm text-ink">
            <input
              type="checkbox"
              className="mt-1"
              checked={state.enabled}
              disabled={(!state.available && !state.enabled) || save.isPending}
              onChange={(event) => change(event.target.checked)}
            />
            <span>
              Ler as páginas escaneadas em segundo plano
              <span className="block text-xs text-ink-soft">
                Ao ligar, os PDFs escaneados que já estão no acervo entram na fila, atrás do resto. Cada página é lida uma vez.
              </span>
            </span>
          </label>

          <fieldset className="mt-4" disabled={save.isPending}>
            <legend className="text-sm font-medium text-ink">Idioma, quando o PDF não diz qual é</legend>
            <p className="mt-1 text-xs text-ink-soft">
              Um PDF que declara o idioma é lido nele. Para os outros, escolha o que o acervo costuma ter: ler com mais de um idioma
              aceita trechos misturados, mas pode errar mais os acentos.
            </p>
            <div className="mt-2 flex flex-wrap gap-x-5 gap-y-2">
              {(state.languages?.length ? state.languages : chosen).map((code) => (
                <label key={code} className="flex items-center gap-2 text-sm text-ink">
                  <input
                    type="checkbox"
                    checked={chosen.includes(code)}
                    disabled={!state.languages?.length || (chosen.length === 1 && chosen[0] === code)}
                    onChange={() => toggle(code)}
                  />
                  {languageName(code)}
                </label>
              ))}
            </div>
          </fieldset>
        </>
      ) : (
        <p className="mt-4 text-sm text-ink-soft">
          {state.enabled ? 'O OCR está ligado.' : 'O OCR está desligado.'} Quem decide é o dono do acervo.
        </p>
      )}
      <ErrorNote>{save.isError && describeError(save.error)}</ErrorNote>
    </Section>
  );
}

function Files({ enabled }) {
  const { data, isLoading, isError } = useOcr({ live: enabled });
  const retry = useRetryOcr();
  const items = data?.data || [];
  return (
    <Section title="PDFs com páginas sem texto" hint="Cada página é lida uma vez e fica guardada. Uma página que falha não é tentada de novo sozinha.">
      {isLoading && <Loading />}
      {isError && <ErrorNote>Não foi possível carregar a lista.</ErrorNote>}
      {!isLoading && !isError && items.length === 0 && <Empty>Nenhum PDF precisa de OCR.</Empty>}
      <ul className="divide-y divide-border-hairline text-[13px]">
        {items.map((item) => {
          const total = item.pagesWithoutText.length;
          const progress = fileProgress(item, enabled);
          const done = total ? Math.min(100, Math.round(((item.read + item.failed) / total) * 100)) : 0;
          return (
            <li key={item.fileId} className="py-3">
              <p className="text-ink">{item.title}</p>
              <p className="text-[12px] text-ink-faint">
                {total === item.pageCount ? `Todas as ${item.pageCount} páginas são imagem` : `${total} de ${item.pageCount} páginas sem texto`}
              </p>
              <p className={`mt-1 text-[12px] ${TONE[progress.tone]}`}>{progress.text}</p>
              {(item.state === 'reading' || (item.read > 0 && item.read + item.failed < total)) && (
                <div className="mt-1 h-1.5 w-full max-w-xs overflow-hidden rounded-full bg-surface-alt" role="progressbar" aria-valuenow={done} aria-valuemin={0} aria-valuemax={100}>
                  <div className="h-full rounded-full bg-brand" style={{ width: `${done}%` }} />
                </div>
              )}
              {item.failed > 0 && !item.state && enabled && (
                <div className="mt-2">
                  <Btn onClick={() => retry.mutate(item.workId)} disabled={retry.isPending}>
                    Tentar de novo {item.failed === 1 ? 'a página que falhou' : `as ${item.failed} páginas que falharam`}
                  </Btn>
                </div>
              )}
            </li>
          );
        })}
      </ul>
      <ErrorNote>{retry.isError && describeError(retry.error)}</ErrorNote>
    </Section>
  );
}

/** OCR: the owner turns it on and picks the language; the staff sees how far it has got and asks for a retry. */
export function OcrTab({ isOwner }) {
  const { data } = useOcrSettings();
  return (
    <div className="flex flex-col gap-5">
      <Settings isOwner={isOwner} />
      <Files enabled={!!data?.enabled} />
    </div>
  );
}
