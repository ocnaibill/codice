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
  return <EmbeddingsForm key={`${data.enabled}-${data.available}-${data.state}`} state={data} />;
}

function EmbeddingsForm({ state }) {
  const [enabled, setEnabled] = useState(state.enabled);
  const save = useSetEmbeddings();
  const change = async (next) => {
    setEnabled(next);
    try {
      await save.mutateAsync({ enabled: next });
    } catch {
      setEnabled(!next);
    }
  };
  return (
    <Section title="Correspondência semântica local" hint="Ajuda a localizar a mesma passagem em traduções distantes. O texto permanece neste servidor.">
      <label className="mt-4 flex items-start gap-3 text-sm text-ink">
        <input
          type="checkbox"
          className="mt-1"
          checked={enabled}
          disabled={!state.available || save.isPending}
          onChange={(event) => change(event.target.checked)}
        />
        <span>Usar o modelo local LaBSE para posições equivalentes</span>
      </label>
      {!state.available ? (
        <ErrorNote>O worker de embeddings não está em execução. Atualize ou reinicie a pilha completa do Códice.</ErrorNote>
      ) : (
        <p className="mt-3 text-xs text-ink-soft">
          {enabled ? (STATE[state.state] || 'Worker disponível.') : 'Desativado. O modelo não será baixado nem usado.'}
          {state.model ? ` Modelo: ${state.model}.` : ''}
        </p>
      )}
      {state.error && <ErrorNote>{state.error}</ErrorNote>}
      <ErrorNote>{save.isError && describeError(save.error)}</ErrorNote>
    </Section>
  );
}
