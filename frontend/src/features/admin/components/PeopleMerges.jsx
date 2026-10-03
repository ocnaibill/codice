import { useState } from 'react';
import { usePeopleMerges, useMergePeople, useDismissPeopleMerge, describeError } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';
import { keyLabel, sourceName } from '../../../lib/authority';

function Side({ person }) {
  return (
    <div className="min-w-0">
      <p className="text-[14px] font-medium text-ink">{person.name}</p>
      <p className="text-[12px] text-ink-faint">
        {person.works === 1 ? '1 obra' : `${person.works} obras`}
        {person.titles?.length > 0 && `: ${person.titles.join(', ')}${person.works > person.titles.length ? '…' : ''}`}
      </p>
      {person.authorities?.length > 0 && <p className="text-[12px] text-ink-faint">Chave: {person.authorities.map(keyLabel).join('; ')}</p>}
      {person.aliases?.length > 0 && <p className="text-[12px] text-ink-faint">Também escrito: {person.aliases.join('; ')}</p>}
    </div>
  );
}

/**
 * People whose names are made of the same words ("Herbert, Frank" and "Frank Herbert", #36). A file that
 * does not say which word is the surname does not say which way round the name goes, so the system only
 * proposes, and an administrator decides. A merge cannot be undone, but nothing that was written is lost:
 * the name that goes becomes another way of writing the one that stays, and searching for it still works.
 */
export function PeopleMerges() {
  const { data, isLoading, isError, refetch, isRefetching } = usePeopleMerges();
  const merge = useMergePeople();
  const dismiss = useDismissPeopleMerge();
  const [merging, setMerging] = useState(null); // { pair, keep, other }
  const pairs = data?.data || [];

  return (
    <Section
      title="Pessoas que talvez sejam a mesma"
      hint="Nomes feitos das mesmas palavras, como “Herbert, Frank” e “Frank Herbert”, ou duas pessoas com a mesma chave de uma fonte de referência (as com chave vêm primeiro). O sistema só sugere: nada é unido sem você decidir."
    >
      {isLoading && <Loading />}
      {isError && <LoadError onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as sugestões.</LoadError>}
      {!isLoading && !isError && pairs.length === 0 && <Empty>Nenhuma sugestão pendente.</Empty>}
      <ul className="divide-y divide-border-hairline">
        {pairs.map((pair) => (
          <li key={pair.id} className="py-4">
            {pair.reason === 'authority' && pair.evidence?.value && (
              <p className="mb-2 text-[12px] font-medium text-ink">
                Mesma chave no {sourceName(pair.evidence.scheme)} ({pair.evidence.value}): muito provavelmente a mesma pessoa.
              </p>
            )}
            <div className="grid gap-3 sm:grid-cols-2">
              <Side person={pair.a} />
              <Side person={pair.b} />
            </div>
            <div className="mt-3 flex flex-wrap gap-2">
              <Btn onClick={() => setMerging({ pair, keep: pair.a, other: pair.b })}>Unir, mantendo “{pair.a.name}”</Btn>
              <Btn onClick={() => setMerging({ pair, keep: pair.b, other: pair.a })}>Unir, mantendo “{pair.b.name}”</Btn>
              <Btn onClick={() => dismiss.mutate(pair.id)} disabled={dismiss.isPending}>Não são a mesma pessoa</Btn>
            </div>
          </li>
        ))}
      </ul>
      <ErrorNote>{dismiss.isError ? describeError(dismiss.error) : merge.isError && describeError(merge.error)}</ErrorNote>

      {merging && (
        <ConfirmDialog
          title="Unir estas pessoas?"
          message={
            <p>
              Todas as obras de “{merging.other.name}” passam a ser de “{merging.keep.name}”, e “{merging.other.name}” fica
              como outro jeito de escrever o nome: a busca continua achando as obras por ele. Isso não pode ser desfeito.
            </p>
          }
          choices={[{ label: 'Unir pessoas', value: true, tone: 'danger' }]}
          onChoose={() => { merge.mutate({ id: merging.pair.id, keep: merging.keep.id }); setMerging(null); }}
          onCancel={() => setMerging(null)}
        />
      )}
    </Section>
  );
}
