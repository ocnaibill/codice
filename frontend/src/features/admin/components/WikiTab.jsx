import { useEffect, useState } from 'react';
import {
  describeError,
  isLiveRun,
  useControlWikiRun,
  useDeleteWikiProvider,
  useGenerateAllWikis,
  useGenerateWiki,
  useSetWikiExclusion,
  useSetWikiProvider,
  useTestWikiProvider,
  useWikiLibrary,
  useWikiProvider,
  useWikiRuns,
} from '../api/admin';
import { KIND_NAMES, RUN_STATE, WIKI_STATE, silent, stagesOf, timing } from '../wikiAdminText';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { ConfirmDialog } from './ConfirmDialog';
import { LoadError } from '../../../components/ui/LoadError';

const FIELD = 'mt-1 w-full rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink placeholder:text-ink-faint';
const CHIP = 'rounded bg-surface-alt px-2 py-0.5 font-mono text-[11px] text-ink-soft';

const PLACEHOLDERS = {
  deepseek: { url: 'https://api.deepseek.com (padrão)', model: 'deepseek-chat' },
  openai: { url: 'https://servidor.exemplo/v1', model: 'nome-do-modelo' },
  ollama: { url: 'http://ollama:11434/v1', model: 'qwen3:32b' },
};

/** The provider, as the owner configures it (DEC-177): kind, address, model and the key, which is typed and never shown again. */
function ProviderForm({ state }) {
  const save = useSetWikiProvider();
  const remove = useDeleteWikiProvider();
  const test = useTestWikiProvider();
  const [form, setForm] = useState({ kind: state.kind || 'deepseek', baseUrl: state.baseUrl || '', model: state.model || '', key: '', clearKey: false });
  const [confirmOff, setConfirmOff] = useState(false);
  useEffect(() => {
    setForm((f) => ({ ...f, kind: state.kind || f.kind, baseUrl: state.baseUrl ?? f.baseUrl, model: state.model ?? f.model }));
  }, [state.kind, state.baseUrl, state.model]);
  const set = (field) => (event) => setForm((f) => ({ ...f, [field]: event.target.type === 'checkbox' ? event.target.checked : event.target.value }));
  const hold = PLACEHOLDERS[form.kind] ?? PLACEHOLDERS.openai;
  const submit = (event) => {
    event.preventDefault();
    test.reset();
    save.mutate(
      { kind: form.kind, baseUrl: form.baseUrl, model: form.model, key: form.key, clearKey: form.clearKey },
      { onSuccess: () => setForm((f) => ({ ...f, key: '', clearKey: false })) }
    );
  };

  return (
    <form onSubmit={submit} className="mt-2 grid gap-4 sm:grid-cols-2" aria-label="Provedor da wiki">
      {state.masterKey === false && (
        <p role="alert" className="rounded-lg bg-warning-soft px-3 py-2 text-[13px] text-warning sm:col-span-2">
          Falta a chave mestra do servidor: defina CODICE_SECRET_KEY (32 caracteres ou mais, como `openssl rand -hex 32`) no ambiente do backend e do worker e reinicie-os.
          Sem ela a chave do provedor não pode ser guardada.
        </p>
      )}
      {state.keyUnreadable && (
        <p role="alert" className="rounded-lg bg-warning-soft px-3 py-2 text-[13px] text-warning sm:col-span-2">
          A chave guardada não abre com a chave mestra atual deste servidor (ela mudou, ou o banco veio de outro). Digite a chave do provedor de novo.
        </p>
      )}
      <label className="text-sm text-ink">
        Tipo
        <select value={form.kind} onChange={set('kind')} className={FIELD}>
          {Object.entries(KIND_NAMES).map(([key, name]) => (
            <option key={key} value={key}>{name}</option>
          ))}
        </select>
      </label>
      <label className="text-sm text-ink">
        Modelo
        <input value={form.model} onChange={set('model')} placeholder={hold.model} className={FIELD} autoComplete="off" spellCheck={false} />
      </label>
      <label className="text-sm text-ink sm:col-span-2">
        Endereço do servidor
        <input value={form.baseUrl} onChange={set('baseUrl')} placeholder={hold.url} className={FIELD} autoComplete="off" spellCheck={false} inputMode="url" />
        <span className="mt-1 block text-xs text-ink-soft">Onde o Códice pede o texto: o caminho até antes de /chat/completions.</span>
      </label>
      <label className="text-sm text-ink sm:col-span-2">
        Chave de API
        <input
          type="password"
          value={form.key}
          onChange={set('key')}
          placeholder={state.keySet ? `Guardada (${state.keyHint || 'termina em ••••'}): deixe em branco para manter` : form.kind === 'ollama' ? 'Opcional no Ollama' : 'Cole a chave'}
          className={FIELD}
          autoComplete="new-password"
          spellCheck={false}
        />
        <span className="mt-1 block text-xs text-ink-soft">Guardada cifrada no banco. Ela não aparece de novo, nem para você.</span>
      </label>
      {state.keySet && (
        <label className="flex items-center gap-2 text-sm text-ink sm:col-span-2">
          <input type="checkbox" checked={form.clearKey} onChange={set('clearKey')} />
          Apagar a chave guardada
        </label>
      )}
      <div className="flex flex-wrap items-center gap-2 sm:col-span-2">
        <Btn tone="primary" type="submit" disabled={save.isPending}>{save.isPending ? 'Salvando…' : 'Salvar'}</Btn>
        {state.configured && (
          <>
            <Btn type="button" onClick={() => test.mutate()} disabled={test.isPending}>{test.isPending ? 'Testando…' : 'Testar'}</Btn>
            <Btn type="button" onClick={() => setConfirmOff(true)} disabled={remove.isPending}>Desligar</Btn>
          </>
        )}
      </div>
      <ErrorNote>{save.isError && describeError(save.error)}</ErrorNote>
      {test.data && (
        <p role="status" className={`text-[13px] sm:col-span-2 ${test.data.ok ? 'text-success' : 'text-danger'}`}>
          {test.data.ok ? `O modelo respondeu em ${(test.data.ms / 1000).toFixed(1)} s${test.data.reply ? `: “${test.data.reply}”` : '.'}` : test.data.problem}
        </p>
      )}
      <ErrorNote>{test.isError && describeError(test.error)}</ErrorNote>
      {confirmOff && (
        <ConfirmDialog
          title="Desligar a geração da wiki?"
          message="Nenhuma geração começa sem provedor: as que estão na fila esperam. As wikis já geradas continuam para leitura. A chave guardada é apagada."
          choices={[{ label: 'Desligar', value: 'off', tone: 'danger' }]}
          onCancel={() => setConfirmOff(false)}
          onChoose={() => remove.mutate(undefined, { onSettled: () => setConfirmOff(false) })}
        />
      )}
    </form>
  );
}

function ProviderSection({ isOwner }) {
  const { data: state, isLoading, isError, error, refetch, isRefetching } = useWikiProvider();
  return (
    <Section
      title="Provedor"
      hint="A wiki é experimental e só é gerada com um provedor de modelos: uma chave de um serviço (como o DeepSeek), um servidor compatível com OpenAI ou um Ollama numa máquina capaz. Modelos pequenos não deram conta no laboratório."
    >
      {isLoading ? (
        <Loading />
      ) : isError || !state ? (
        <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar o provedor da wiki.</LoadError>
      ) : (
        <>
          {state.configured ? (
            <p className="text-sm text-ink">
              Em uso: <strong>{KIND_NAMES[state.kind] ?? state.kind}</strong> · <span className="font-mono text-[13px]">{state.model}</span>
              {state.host && <span className="text-ink-soft"> · {state.host}</span>}
              <span className={`ml-2 ${CHIP}`}>{state.remote ? 'remoto: o texto sai do servidor' : 'na sua rede'}</span>
            </p>
          ) : (
            <p className="text-sm text-ink-soft">Nenhum provedor configurado: nenhuma wiki é gerada.{isOwner ? '' : ' Só o dono do acervo pode configurar.'}</p>
          )}
          {isOwner && state.configured && state.policy && (
            <p className="mt-2 max-w-3xl rounded-lg bg-surface/60 px-3 py-2 text-[13px] text-ink-soft">{state.policy}</p>
          )}
          {isOwner && <ProviderForm state={state} />}
        </>
      )}
    </Section>
  );
}

function Stages({ run }) {
  return (
    <ol className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[12px]" aria-label="Etapas">
      {stagesOf(run).map((s) => (
        <li key={s.key} aria-current={s.state === 'current' ? 'step' : undefined} className={s.state === 'done' ? 'text-success' : s.state === 'current' ? 'font-semibold text-brand' : 'text-ink-faint'}>
          {s.state === 'done' ? '✓ ' : s.state === 'current' ? '● ' : '○ '}
          {s.label}
          {s.count && <span className="ml-1 font-mono text-[11px]">{s.count}</span>}
        </li>
      ))}
    </ol>
  );
}

function RunRow({ run }) {
  const control = useControlWikiRun();
  const time = timing(run);
  const errors = Array.isArray(run.errors) ? run.errors.slice(-3) : [];
  const act = (action) => control.mutate({ id: run.id, action });
  return (
    <li className="py-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-medium text-ink">
            {run.title}
            <span className="ml-2 text-xs font-normal text-ink-soft">{run.scope.kind === 'series' ? 'série' : 'obra'}</span>
          </p>
          <p className="mt-0.5 flex flex-wrap gap-x-2 text-xs text-ink-soft">
            <span className={CHIP}>{RUN_STATE[run.state] ?? run.state}{run.position ? ` · ${run.position}º` : ''}</span>
            {run.origin === 'batch' && <span>pelo “gerar para todas”</span>}
            {run.origin === 'new_book' && <span>livro novo na série</span>}
            {run.place && <span>em {run.place}</span>}
            {time && <span>{time.elapsed}{time.left ? ` · ≈ ${time.left} restantes` : ''}</span>}
            {run.resumes > 0 && <span>retomada {run.resumes}×</span>}
            {run.pauseRequested && <span>pausando no próximo passo…</span>}
            {run.cancelRequested && <span>cancelando no próximo passo…</span>}
          </p>
        </div>
        {isLiveRun(run) && (
          <div className="flex gap-2">
            {(run.state === 'queued' || run.state === 'running') && !run.pauseRequested && <Btn onClick={() => act('pause')} disabled={control.isPending}>Pausar</Btn>}
            {run.state === 'paused' && <Btn tone="primary" onClick={() => act('resume')} disabled={control.isPending}>Retomar</Btn>}
            {!run.cancelRequested && <Btn onClick={() => act('cancel')} disabled={control.isPending}>Cancelar</Btn>}
          </div>
        )}
      </div>
      {(run.state !== 'queued' || run.stage) && <Stages run={run} />}
      {silent(run) && <ErrorNote>O worker não dá notícia há mais de cinco minutos: confira se ele está em execução.</ErrorNote>}
      {errors.length > 0 && (
        <ul className="mt-2 space-y-0.5 text-[12px] text-danger">
          {errors.map((e, i) => (
            <li key={i}>{e.message}</li>
          ))}
        </ul>
      )}
      <ErrorNote>{control.isError && describeError(control.error)}</ErrorNote>
    </li>
  );
}

function RunsSection() {
  const { data, isLoading, isError, error, refetch, isRefetching } = useWikiRuns();
  const runs = data?.runs ?? [];
  const live = runs.filter(isLiveRun);
  const past = runs.filter((r) => !isLiveRun(r));
  return (
    <Section title="Andamento" hint="Uma geração por vez, na ordem da fila. Pausar ou cancelar a que está rodando vale no próximo passo dela; retomar não refaz o que já terminou.">
      {isLoading ? (
        <Loading />
      ) : isError ? (
        <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as gerações.</LoadError>
      ) : (
        <>
          {live.length === 0 ? <Empty>Nenhuma geração na fila.</Empty> : <ul className="divide-y divide-border-hairline">{live.map((r) => <RunRow key={r.id} run={r} />)}</ul>}
          {past.length > 0 && (
            <details className="mt-3">
              <summary className="cursor-pointer text-[13px] text-ink-soft">Últimas terminadas ({past.length})</summary>
              <ul className="divide-y divide-border-hairline">{past.map((r) => <RunRow key={r.id} run={r} />)}</ul>
            </details>
          )}
        </>
      )}
    </Section>
  );
}

const FILTERS = [
  ['', 'Todas'],
  ['without', 'Sem wiki'],
  ['with', 'Com wiki'],
  ['live', 'Na fila ou gerando'],
  ['excluded', 'Exceções'],
];

function LibraryRow({ item, canGenerate }) {
  const generate = useGenerateWiki();
  const exclude = useSetWikiExclusion();
  const wikiState = item.wiki ? (item.wiki.review === 'pending' ? 'A revisar' : WIKI_STATE[item.wiki.state] ?? item.wiki.state) : 'Sem wiki';
  return (
    <li className="flex flex-wrap items-center justify-between gap-3 py-3">
      <div className="min-w-0">
        <p className="text-sm font-medium text-ink">{item.title}</p>
        <p className="mt-0.5 flex flex-wrap gap-x-2 text-xs text-ink-soft">
          <span>{item.kind === 'series' ? `série · ${item.books} ${item.books === 1 ? 'livro' : 'livros'}` : 'obra'}</span>
          {item.readable < item.books && <span>{item.readable} com texto para ler</span>}
          <span className={CHIP}>{wikiState}</span>
          {item.run && <span className={CHIP}>{RUN_STATE[item.run.state] ?? item.run.state}</span>}
        </p>
        <ErrorNote>{(generate.isError && describeError(generate.error)) || (exclude.isError && describeError(exclude.error))}</ErrorNote>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-[13px] text-ink-soft">
          <input
            type="checkbox"
            checked={item.excluded}
            disabled={exclude.isPending}
            onChange={(e) => exclude.mutate({ kind: item.kind, id: item.id, excluded: e.target.checked })}
          />
          Não gerar wiki
        </label>
        <Btn onClick={() => generate.mutate({ kind: item.kind, id: item.id })} disabled={!canGenerate || !!item.run || generate.isPending}>
          {item.wiki ? 'Gerar de novo' : 'Gerar'}
        </Btn>
      </div>
    </li>
  );
}

function LibrarySection({ canGenerate }) {
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState('');
  const [page, setPage] = useState(1);
  const [confirmAll, setConfirmAll] = useState(false);
  const all = useGenerateAllWikis();
  const { data, isLoading, isError, error, refetch, isRefetching } = useWikiLibrary({ q, filter, page });
  const items = data?.items ?? [];
  const pages = Math.max(1, Math.ceil((data?.total ?? 0) / 50));
  useEffect(() => setPage(1), [q, filter]);
  return (
    <Section
      title="Obras e séries"
      hint="Só livros em EPUB com o texto lido pelo Códice podem ter wiki; uma série tem uma wiki só, com todos os livros. As marcadas “Não gerar wiki” ficam fora do “gerar para todas” e da atualização de livro novo, mas podem ser geradas aqui uma a uma."
      actions={
        <Btn tone="primary" onClick={() => setConfirmAll(true)} disabled={!canGenerate || all.isPending}>
          Gerar para todas que ainda não têm wiki
        </Btn>
      }
    >
      {all.data && <p role="status" className="mb-3 text-[13px] text-success">{all.data.queued === 0 ? 'Nada novo para a fila: todas já têm wiki, estão na fila ou são exceções.' : `${all.data.queued} na fila.`}</p>}
      <ErrorNote>{all.isError && describeError(all.error)}</ErrorNote>
      {!canGenerate && <p className="mb-3 text-[13px] text-ink-soft">Para gerar, configure o provedor acima.</p>}
      <div className="flex flex-wrap items-end gap-3">
        <label className="w-full text-sm text-ink lg:w-auto lg:min-w-[16rem] lg:flex-1">
          Procurar
          <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Título da obra ou da série" className={FIELD} />
        </label>
        <div role="group" aria-label="Mostrar" className="flex flex-wrap gap-2">
          {FILTERS.map(([key, label]) => (
            <button
              key={key}
              type="button"
              aria-pressed={filter === key}
              onClick={() => setFilter(key)}
              className="min-h-9 rounded-full border border-border-hairline bg-white px-3 text-[13px] text-ink hover:bg-surface-alt aria-pressed:border-brand aria-pressed:bg-brand aria-pressed:text-white"
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      {isLoading ? (
        <Loading />
      ) : isError ? (
        <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as obras.</LoadError>
      ) : items.length === 0 ? (
        <Empty>Nada aqui.</Empty>
      ) : (
        <ul className="mt-2 divide-y divide-border-hairline">
          {items.map((item) => (
            <LibraryRow key={`${item.kind}-${item.id}`} item={item} canGenerate={canGenerate} />
          ))}
        </ul>
      )}
      {pages > 1 && (
        <div className="mt-3 flex items-center gap-3 text-[13px] text-ink-soft">
          <Btn onClick={() => setPage((p) => p - 1)} disabled={page <= 1}>Anterior</Btn>
          <span>Página {page} de {pages}</span>
          <Btn onClick={() => setPage((p) => p + 1)} disabled={page >= pages}>Próxima</Btn>
        </div>
      )}
      {confirmAll && (
        <ConfirmDialog
          title="Gerar a wiki de todas?"
          message="Entram na fila todas as obras e séries que ainda não têm wiki, menos as exceções. Elas rodam uma por vez; cada uma manda o texto dos livros ao provedor e pode levar de minutos a horas."
          choices={[{ label: 'Colocar na fila', value: 'all', tone: 'primary' }]}
          onCancel={() => setConfirmAll(false)}
          onChoose={() => all.mutate(undefined, { onSettled: () => setConfirmAll(false) })}
        />
      )}
    </Section>
  );
}

/** The tab of the wiki in the administration (DEC-185): the provider, the queue of generations, and the works and series. */
export function WikiTab({ isOwner }) {
  const provider = useWikiProvider().data;
  return (
    <div className="flex flex-col gap-6">
      <ProviderSection isOwner={isOwner} />
      <RunsSection />
      <LibrarySection canGenerate={!!provider?.configured} />
    </div>
  );
}
