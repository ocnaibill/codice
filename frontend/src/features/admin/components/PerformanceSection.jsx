import { useEffect, useState } from 'react';
import { describeError, usePerformance, useSetPerformance } from '../api/admin';
import { LoadError } from '../../../components/ui/LoadError';
import { Notice } from '../../../components/ui/Notice';
import { PermissionNote } from '../../../components/ui/PermissionNote';
import { formatBytes } from '../format';
import { Btn, ErrorNote, Loading, Section } from './ui';

// What each setting is, in the words of the person who runs the server. The order is the one of the server's `fields`.
const FIELDS = [
  {
    key: 'catalogReads',
    label: 'Leituras pesadas das telas',
    help: 'Quantas listas, contagens e buscas o servidor monta ao mesmo tempo. O resto espera um pouco na fila. Se o app ficar lento ou parar de responder durante uma importação grande, baixe; numa máquina forte, pode subir.',
  },
  {
    key: 'dedupeJobs',
    label: 'Comparações de duplicatas ao mesmo tempo',
    help: 'Quantas obras o servidor compara por vez para propor possíveis duplicatas. É trabalho do banco de dados: mais de uma acelera a fila, e pesa nas telas.',
  },
  {
    key: 'ocrPages',
    label: 'Páginas lidas ao mesmo tempo pelo OCR',
    help: 'Quantas páginas de um PDF escaneado o OCR lê juntas. É o que mais encurta a fila do OCR. Cada página a mais usa um núcleo e cerca de 340 MiB de memória.',
  },
  {
    key: 'ocrThreads',
    label: 'Núcleos por página do OCR',
    help: 'Quantos núcleos o motor usa para ler uma única página. Passar de 1 ou 2 costuma acelerar pouco: é melhor ler mais páginas ao mesmo tempo.',
  },
];

const GIB = 1024 ** 3;

function warningText(code, values, machine) {
  if (code === 'ocrCores') {
    return `O OCR pediria ${values.ocrPages * values.ocrThreads} núcleos (${values.ocrPages} páginas com ${values.ocrThreads} cada) e a máquina tem ${machine.cores}: tudo passa a disputar o processador, e as telas ficam mais lentas.`;
  }
  if (code === 'ocrMemory') {
    return `O OCR sozinho ocuparia cerca de ${formatBytes(values.ocrPages * 340 * 1024 ** 2)}, mais da metade da memória da máquina (${formatBytes(machine.memoryBytes)}).`;
  }
  return '';
}

/**
 * "Desempenho" (Sistema): how much the server does at once, for the owner to tune without editing the .env or restarting
 * anything. The change holds from the next job: what is running finishes as it was. The staff sees the values; only the owner
 * changes them.
 */
export function PerformanceSection({ isOwner = false }) {
  const { data, isLoading, isError, error, refetch, isRefetching } = usePerformance();
  const save = useSetPerformance();
  const [draft, setDraft] = useState({});

  // What is in use is what the server says; the draft is only what was typed and not saved yet.
  useEffect(() => {
    if (save.isSuccess) setDraft({});
  }, [save.isSuccess, save.data]);

  if (isLoading) return <Section title="Desempenho"><Loading /></Section>;
  if (isError || !data) {
    return (
      <Section title="Desempenho">
        <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar os ajustes de desempenho.</LoadError>
      </Section>
    );
  }

  const shown = (key) => (draft[key] !== undefined ? draft[key] : data.values[key]);
  const changes = {};
  for (const { key } of FIELDS) {
    if (draft[key] !== undefined && draft[key] !== data.values[key]) {
      changes[key] = draft[key] === data.defaults[key] ? null : draft[key];
    }
  }
  const dirty = Object.keys(changes).length > 0;
  const valid = FIELDS.every(({ key }) => {
    const v = shown(key);
    const limit = data.limits[key];
    return Number.isInteger(v) && v >= limit.min && v <= limit.max;
  });
  const preview = Object.fromEntries(FIELDS.map(({ key }) => [key, Number.isInteger(shown(key)) ? shown(key) : data.values[key]]));
  // The same two rules as the server, for the numbers typed (without typing anything, they are the server's own).
  const warnings = previewWarnings(preview, data.machine);
  const failure = save.isError ? describeError(save.error) : '';
  const ocr = data.ocr;
  const ocrBehind = ocr.reported && (ocr.pages !== data.values.ocrPages || ocr.threads !== data.values.ocrThreads);

  return (
    <Section
      title="Desempenho"
      hint="Quanto o servidor faz ao mesmo tempo. Vale na hora, sem reiniciar nada: o que já está rodando termina como estava, e a mudança vale para o próximo trabalho. Um número maior termina a fila mais cedo e usa mais da máquina."
    >
      <p className="mb-3 text-[13px] text-ink-soft">
        Esta máquina tem <strong className="font-medium text-ink">{data.machine.cores}</strong> {data.machine.cores === 1 ? 'núcleo' : 'núcleos'}
        {data.machine.memoryBytes > 0 && <> e <strong className="font-medium text-ink">{(data.machine.memoryBytes / GIB).toFixed(data.machine.memoryBytes >= 10 * GIB ? 0 : 1).replace('.', ',')} GiB</strong> de memória</>}
        . Os valores não passam do que ela comporta.
      </p>
      {!isOwner && <PermissionNote className="mb-3">Só o dono do acervo muda estes valores. Você os vê como estão.</PermissionNote>}
      <ul className="divide-y divide-border-hairline">
        {FIELDS.map(({ key, label, help }) => {
          const limit = data.limits[key];
          const inUse = data.values[key];
          const isDefault = inUse === data.defaults[key];
          return (
            <li key={key} className="flex flex-wrap items-start justify-between gap-3 py-3">
              <div className="min-w-0 flex-1 basis-64">
                <label htmlFor={`perf-${key}`} className="text-[13px] font-medium text-ink">{label}</label>
                <p className="mt-0.5 text-[12px] text-ink-soft">{help}</p>
              </div>
              <div className="flex shrink-0 flex-col items-end gap-1">
                {isOwner ? (
                  <input
                    id={`perf-${key}`}
                    type="number"
                    inputMode="numeric"
                    min={limit.min}
                    max={limit.max}
                    step={1}
                    value={Number.isNaN(shown(key)) || shown(key) === undefined ? '' : shown(key)}
                    onChange={(event) => setDraft((d) => ({ ...d, [key]: event.target.value === '' ? NaN : Number(event.target.value) }))}
                    aria-describedby={`perf-${key}-range`}
                    className="w-20 rounded bg-surface px-3 py-2 text-right text-[14px] text-ink"
                  />
                ) : (
                  <output id={`perf-${key}`} className="text-[14px] font-medium text-ink">{inUse}</output>
                )}
                <span id={`perf-${key}-range`} className="text-[11px] text-ink-faint">de {limit.min} a {limit.max}</span>
                <span className="text-[11px] text-ink-faint">
                  {isDefault ? `padrão: ${data.defaults[key]}` : `padrão: ${data.defaults[key]}, escolhido: ${inUse}`}
                  {isOwner && !isDefault && (
                    <button type="button" className="ml-2 text-brand underline" onClick={() => save.mutate({ [key]: null })} disabled={save.isPending}>
                      voltar ao padrão
                    </button>
                  )}
                </span>
              </div>
            </li>
          );
        })}
      </ul>
      {ocr.reported && (
        <p className="mt-3 text-[12px] text-ink-faint" role="status">
          O serviço de OCR está usando agora {ocr.pages} {ocr.pages === 1 ? 'página' : 'páginas'} ao mesmo tempo, com {ocr.threads} {ocr.threads === 1 ? 'núcleo' : 'núcleos'} cada.
          {ocrBehind && ' O que você escolheu vale a partir do próximo PDF.'}
        </p>
      )}
      {warnings.length > 0 && (
        <Notice tone="warning" className="mt-3" title="Isso pede mais do que a máquina tem">
          {warnings.map((code) => <p key={code}>{warningText(code, preview, data.machine)}</p>)}
        </Notice>
      )}
      <ErrorNote>{failure}</ErrorNote>
      {isOwner && (
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <Btn tone="primary" disabled={!dirty || !valid || save.isPending} onClick={() => save.mutate(changes)}>
            {save.isPending ? 'Salvando…' : 'Salvar'}
          </Btn>
          {dirty && !save.isPending && <Btn onClick={() => setDraft({})}>Descartar</Btn>}
          {!valid && <span role="alert" className="text-[12px] text-danger">Use números inteiros dentro das faixas.</span>}
          {save.isSuccess && !dirty && <span role="status" className="text-[12px] text-ink-soft">Salvo. Vale a partir do próximo trabalho.</span>}
        </div>
      )}
    </Section>
  );
}

// What the screen warns of while the owner types, before saving: the same two rules as the server, for the numbers typed.
function previewWarnings(values, machine) {
  const out = [];
  if (machine.cores > 0 && values.ocrPages * values.ocrThreads > machine.cores) out.push('ocrCores');
  if (machine.memoryBytes > 0 && values.ocrPages * 340 * 1024 ** 2 > machine.memoryBytes / 2) out.push('ocrMemory');
  return out;
}
