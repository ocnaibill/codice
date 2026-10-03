import { useMetadataProviders, useSuggestionQueue } from '../api/admin';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { FIELD_LABELS } from '../../../lib/suggestionFields';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';

const fieldNames = (fields) => fields.map((f) => FIELD_LABELS[f] || f).join(', ');

/**
 * The works that have suggestions from the providers nobody decided yet (#70): the ones waiting the longest
 * first. Reviewing one opens the metadata of the work, where each suggestion is accepted or rejected; the
 * work leaves the queue when its last suggestion is decided.
 */
export function SuggestionsTab({ isOwner, onOpenProviders }) {
  const { data, isLoading, isError, refetch, isRefetching } = useSuggestionQueue();
  const providers = useMetadataProviders().data?.data;
  // Known to be off, not merely not loaded yet: a banner that flashes on every open would be noise.
  const allOff = !!providers && providers.length > 0 && providers.every((p) => !p.enabled);
  const openMetadata = useGlobalStore((state) => state.openMetadata);
  const works = data?.data || [];
  const total = data?.total ?? works.length;

  return (
    <Section
      title="Sugestões dos provedores"
      hint="Obras com sugestões de metadados esperando decisão, as mais antigas primeiro. Nada muda até você aceitar."
    >
      {allOff && (
        <p role="status" className="mb-3 rounded-lg bg-surface-alt p-3 text-[13px] text-ink-soft">
          Nenhum provedor externo está ligado, então não chegam sugestões novas.{' '}
          {isOwner ? (
            <button onClick={onOpenProviders} className="font-medium text-brand hover:underline">Escolher os provedores</button>
          ) : (
            'Só o owner liga os provedores.'
          )}
        </p>
      )}
      {isLoading && <Loading />}
      {isError && <LoadError onRetry={refetch} retrying={isRefetching}>Não foi possível carregar a fila.</LoadError>}
      {!isLoading && !isError && works.length === 0 && <Empty>Nenhuma obra com sugestões esperando.</Empty>}
      <ul className="divide-y divide-border-hairline">
        {works.map((work) => (
          <li key={work.workId} className="flex flex-wrap items-center justify-between gap-3 py-3">
            <div className="min-w-0">
              <p className="truncate text-[14px] font-medium text-ink">{work.title}</p>
              <p className="truncate text-[12px] text-ink-faint">{work.author || 'Sem autor'}</p>
              <p className="text-[12px] text-ink-soft">
                {work.pending === 1 ? '1 sugestão' : `${work.pending} sugestões`}: {fieldNames(work.fields)}
              </p>
            </div>
            <Btn tone="primary" onClick={() => openMetadata(work.workId, 'suggestions')}>Revisar</Btn>
          </li>
        ))}
      </ul>
      {total > works.length && (
        <p className="mt-3 text-[12px] text-ink-faint">
          Mostrando {works.length} de {total} obras; as outras aparecem conforme estas são decididas.
        </p>
      )}
    </Section>
  );
}
