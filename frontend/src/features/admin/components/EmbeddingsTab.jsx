import { useState } from 'react';
import { describeError, useModels, useRereadComicOcr, useSetComicOcr, useSetEmbeddings } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';

const NONE = 'none';

const STATE = {
  off: { text: 'Desativado', tone: 'text-ink-faint' },
  service_missing: { text: 'Serviço parado', tone: 'text-danger' },
  preparing: { text: 'Preparando o modelo…', tone: 'text-ink-soft' },
  downloading: { text: 'Baixando', tone: 'text-ink-soft' },
  waiting: { text: 'Pronto para trabalhar, esperando arquivos', tone: 'text-ink-soft' },
  ready: { text: 'Pronto, em uso', tone: 'text-success' },
  error: { text: 'Com erro', tone: 'text-danger' },
};

const SERVICE_MISSING = {
  semantic: 'O worker de embeddings não está em execução. Atualize ou reinicie a pilha completa do Códice.',
  comic_ocr: 'O serviço de OCR de quadrinhos não está em execução. Suba o serviço comic-ocr da pilha completa do Códice.',
};

/** One use of a model: the choices are "none" and the models that do it, one at a time. */
function Choice({ legend, hint, name, kind, models, chosen, serviceUp, busy, onChoose }) {
  const options = models.filter((m) => m.kind === kind);
  return (
    <fieldset className="mt-5 first:mt-0" disabled={busy}>
      <legend className="text-sm font-medium text-ink">{legend}</legend>
      <p className="mt-1 text-xs text-ink-soft">{hint}</p>
      <div className="mt-3 space-y-3">
        <label className="flex items-start gap-3 text-sm text-ink">
          <input type="radio" name={name} className="mt-1" checked={chosen === NONE} onChange={() => onChoose(NONE)} />
          <span>
            <span className="block font-medium">Nenhum</span>
            <span className="block text-xs text-ink-soft">Não usar. O modelo não é baixado nem carregado.</span>
          </span>
        </label>
        {options.map((m) => (
          <label key={m.id} className="flex items-start gap-3 text-sm text-ink">
            <input
              type="radio"
              name={name}
              className="mt-1"
              checked={chosen === m.id}
              disabled={!serviceUp && chosen !== m.id}
              onChange={() => onChoose(m.id)}
            />
            <span>
              <span className="block font-medium">{m.name}</span>
              <span className="block text-xs text-ink-soft">{m.scope} · download de aproximadamente {m.sizeMB} MB</span>
            </span>
          </label>
        ))}
      </div>
      {!serviceUp && <ErrorNote>{SERVICE_MISSING[kind]}</ErrorNote>}
    </fieldset>
  );
}

function Progress({ value, label }) {
  return (
    <div className="mt-1 h-1.5 w-40 overflow-hidden rounded bg-surface-alt" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={value}>
      <div className="h-full bg-brand" style={{ width: `${value}%` }} />
    </div>
  );
}

function ModelsTable({ models }) {
  return (
    <ul className="divide-y divide-border-hairline" aria-label="Modelos locais">
      {models.map((m) => {
        const state = STATE[m.state] ?? { text: m.state, tone: 'text-ink-soft' };
        const downloading = m.state === 'downloading' && m.progress != null;
        return (
          <li key={m.id} className="grid gap-1 py-3 text-[13px] sm:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)] sm:gap-4">
            <div className="min-w-0">
              <span className="block font-medium text-ink">{m.name}</span>
              <span className="block text-xs text-ink-soft">{m.kind === 'comic_ocr' ? 'Lê quadrinhos e mangás' : 'Correspondência semântica'} · {m.sizeMB} MB</span>
            </div>
            <div>
              <span className={state.tone}>
                {state.text}
                {downloading ? ` ${m.progress}%` : ''}
              </span>
              {downloading && <Progress value={m.progress} label={`Download de ${m.name}`} />}
              {m.error && <span className="block text-xs text-danger">{m.error}</span>}
            </div>
            <div className="text-ink-soft">
              {m.usage.count.toLocaleString('pt-BR')} {m.usage.unit}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

/** The pages that a model other than the chosen one read: the owner decides whether they are read again (DEC-176). */
function RereadNote({ pages, modelName, serviceUp, mutation }) {
  const [asking, setAsking] = useState(false);
  const count = pages.toLocaleString('pt-BR');
  const noun = pages === 1 ? 'página foi lida' : 'páginas foram lidas';
  return (
    <div className="mt-3 rounded-lg bg-surface-alt px-3 py-2 text-xs text-ink-soft">
      <p>
        {count} {noun} com outro modelo. O texto delas continua na busca até a nova leitura.
      </p>
      <button
        type="button"
        onClick={() => setAsking(true)}
        disabled={!serviceUp || mutation.isPending}
        className="mt-2 min-h-11 rounded-full border border-border-hairline bg-white px-4 text-[13px] font-medium text-ink transition-[background-color,transform] duration-150 hover:bg-surface active:scale-95 disabled:cursor-not-allowed disabled:opacity-40"
      >
        Reler {count} {pages === 1 ? 'página' : 'páginas'} com este modelo
      </button>
      {mutation.isSuccess && !mutation.isPending && <p role="status" className="mt-2">As páginas entraram na fila para serem lidas de novo.</p>}
      {asking && (
        <ConfirmDialog
          title="Reler as páginas com este modelo?"
          message={
            <p>
              {count} {pages === 1 ? 'página será lida' : 'páginas serão lidas'} de novo, em segundo plano, com {modelName}. Cada página leva de alguns segundos
              a mais de um minuto, e o texto que está na busca só é trocado quando a leitura nova termina.
            </p>
          }
          choices={[{ label: 'Reler as páginas', value: true, tone: 'primary' }]}
          onChoose={() => {
            setAsking(false);
            mutation.mutate();
          }}
          onCancel={() => setAsking(false)}
        />
      )}
    </div>
  );
}

export function EmbeddingsTab() {
  const { data, isLoading, isError, error, refetch, isRefetching } = useModels();
  const semantic = useSetEmbeddings();
  const comic = useSetComicOcr();
  const reread = useRereadComicOcr();
  if (isLoading) return <Loading />;
  if (isError || !data) return <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar os modelos.</LoadError>;

  const chooseSemantic = (id) =>
    semantic.mutate(id === NONE ? { enabled: false, model: lastSemantic(data) } : { enabled: true, model: id });
  const chooseComic = (id) => comic.mutate(id);

  return (
    <div className="space-y-5">
      <Section
        title="IA local"
        hint="Modelos que rodam neste servidor, só quando você escolhe. O texto dos livros e as páginas dos quadrinhos não saem dele; para baixar um modelo, o servidor se conecta uma vez a um host de modelos."
      >
        <Choice
          legend="Correspondência semântica"
          hint="Ajuda a localizar a mesma passagem em traduções distantes."
          name="semantic-model"
          kind="semantic"
          models={data.models}
          chosen={data.controls.semantic}
          serviceUp={data.services.semantic}
          busy={semantic.isPending}
          onChoose={chooseSemantic}
        />
        <ErrorNote>{semantic.isError && describeError(semantic.error)}</ErrorNote>
        <Choice
          legend="OCR de quadrinhos e mangás"
          hint="Lê o texto dos balões das imagens de CBZ e CBR, uma vez, em segundo plano. O texto reconhecido pode ter erros. O OCR dos PDFs escaneados é outro, na aba OCR."
          name="comic-ocr-model"
          kind="comic_ocr"
          models={data.models}
          chosen={data.controls.comicOcr}
          serviceUp={data.services.comicOcr}
          busy={comic.isPending}
          onChoose={chooseComic}
        />
        <ErrorNote>{comic.isError && describeError(comic.error)}</ErrorNote>
        {data.controls.comicOcr !== NONE && data.comicOcr?.otherModelPages > 0 && (
          <RereadNote
            pages={data.comicOcr.otherModelPages}
            modelName={data.models.find((m) => m.id === data.controls.comicOcr)?.name ?? 'o modelo escolhido'}
            serviceUp={data.services.comicOcr}
            mutation={reread}
          />
        )}
        <ErrorNote>{reread.isError && describeError(reread.error)}</ErrorNote>
      </Section>
      <Section title="Modelos" hint="O que está escolhido, em que ponto está o download e quanto cada modelo já foi usado.">
        <ModelsTable models={data.models} />
      </Section>
    </div>
  );
}

/** The semantic model to keep when it is turned off: the one that was chosen, so that turning it on again gives the same. */
function lastSemantic(data) {
  const chosen = data.models.find((m) => m.kind === 'semantic' && m.selected);
  return chosen ? chosen.id : data.models.find((m) => m.kind === 'semantic')?.id;
}
