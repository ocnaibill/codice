import { useState } from 'react';
import { describeError, useEmbeddings, useSetEmbeddings } from '../api/admin';
import { ErrorNote, Loading, Section } from './ui';

const STATE = {
  idle: 'Aguardando texto para processar.',
  preparing: 'Baixando ou preparando o modelo local…',
  ready: 'Modelo local pronto.',
  error: 'O worker não conseguiu preparar o modelo.',
};

export function EmbeddingsTab() {
  const { data, isLoading, isError } = useEmbeddings();
  if (isLoading) return <Loading />;
  if (isError || !data) return <ErrorNote>Não foi possível carregar a configuração.</ErrorNote>;
  return <EmbeddingsForm key={`${data.enabled}-${data.available}-${data.state}-${data.model}`} state={data} />;
}

function EmbeddingsForm({ state }) {
  const [enabled, setEnabled] = useState(state.enabled);
  const [model, setModel] = useState(state.model);
  const save = useSetEmbeddings();
  const change = async (next, nextModel = model) => {
    const previousEnabled = enabled;
    const previousModel = model;
    setEnabled(next);
    setModel(nextModel);
    try {
      await save.mutateAsync({ enabled: next, model: nextModel });
    } catch {
      setEnabled(previousEnabled);
      setModel(previousModel);
    }
  };
  return (
    <Section title="Correspondência semântica local" hint="Ajuda a localizar a mesma passagem em traduções distantes. O texto permanece neste servidor.">
      <fieldset className="mt-4 space-y-3" disabled={save.isPending}>
        <legend className="text-sm font-medium text-ink">Modelo local</legend>
        {(state.models || []).map((option) => (
          <label key={option.id} className="flex items-start gap-3 text-sm text-ink">
            <input
              type="radio"
              name="embedding-model"
              className="mt-1"
              checked={model === option.id}
              onChange={() => change(enabled, option.id)}
            />
            <span>
              <span className="block font-medium">{option.name}</span>
              <span className="block text-xs text-ink-soft">{option.scope} · download de aproximadamente {option.downloadMB} MB</span>
            </span>
          </label>
        ))}
      </fieldset>
      <label className="mt-4 flex items-start gap-3 text-sm text-ink">
        <input
          type="checkbox"
          className="mt-1"
          checked={enabled}
          disabled={!state.available || save.isPending}
          onChange={(event) => change(event.target.checked)}
        />
        <span>Usar correspondência semântica para posições equivalentes</span>
      </label>
      {!state.available ? (
        <ErrorNote>O worker de embeddings não está em execução. Atualize ou reinicie a pilha completa do Códice.</ErrorNote>
      ) : (
        <p className="mt-3 text-xs text-ink-soft">
          {enabled ? (STATE[state.state] || 'Worker disponível.') : 'Desativado. O modelo não será baixado nem usado.'}
          {enabled && state.activeModel && state.activeModel !== model ? ' O worker está trocando de modelo.' : ''}
        </p>
      )}
      {state.error && <ErrorNote>{state.error}</ErrorNote>}
      <ErrorNote>{save.isError && describeError(save.error)}</ErrorNote>
    </Section>
  );
}
