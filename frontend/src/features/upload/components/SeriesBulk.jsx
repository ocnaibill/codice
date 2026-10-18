import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from '../../../components/ui/toast';
import { refreshLibrary } from '../../../lib/refreshLibrary';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { describeUploadError, isInSeries, uploadBook } from '../api/useUploadBook';
import { filesFromDrop } from '../dropped';
import {
  MAX_SERIES_FILES, SERIES_CONCURRENCY, deriveRows, folderOf, guessSeries, listNumbers, pickSeriesFiles, readNumber, runPool, sortRows, summarize,
} from '../seriesFiles';

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

const KINDS = [['manga', 'Mangá'], ['comic', 'Quadrinho']];
const UNITS = [['chapter', 'Capítulo'], ['volume', 'Volume'], ['auto', 'Cada um pelo nome']];

// What a row is going through once the sending begins; before it, its situation in the series is what is shown.
const SENT = { queued: 'queued', uploading: 'uploading', done: 'done', skipped: 'skipped', duplicate: 'duplicate', error: 'error', cancelled: 'cancelled' };

const SITUATION = {
  new: { text: 'novo', tone: 'text-success' },
  repeated: { text: 'número repetido', tone: 'text-danger' },
  missing: { text: 'sem número: dê um ou tire', tone: 'text-warning' },
};

function situationOf(row) {
  switch (row.status) {
    case SENT.uploading: return { text: `Enviando… ${row.progress}%`, tone: 'text-ink-soft' };
    case SENT.done: return { text: 'Enviado', tone: 'text-success' };
    case SENT.skipped: return { text: row.message, tone: 'text-warning' };
    case SENT.duplicate: return { text: row.message, tone: 'text-warning' };
    case SENT.error: return { text: row.message, tone: 'text-danger' };
    case SENT.cancelled: return { text: 'Cancelado', tone: 'text-ink-soft' };
    default: return SITUATION[row.situation];
  }
}

function Choice({ legend, name, value, options, onChange, disabled }) {
  return (
    <fieldset className="flex min-w-0 flex-col gap-1.5" disabled={disabled}>
      <legend className="mb-1.5 font-mono text-[10px] uppercase tracking-[0.14em] text-ink-soft">{legend}</legend>
      <div className="flex flex-wrap gap-1.5">
        {options.map(([id, label]) => (
          <label
            key={id}
            className={`flex min-h-9 cursor-pointer items-center rounded-lg border px-3 text-[13px] transition-colors focus-within:ring-2 focus-within:ring-brand/20 ${
              value === id ? 'border-brand bg-brand/10 font-semibold text-brand' : 'border-border-hairline text-ink hover:bg-surface-alt'
            } ${disabled ? 'opacity-60' : ''}`}
          >
            <input type="radio" name={name} value={id} checked={value === id} onChange={() => onChange(id)} className="sr-only" />
            {label}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

/**
 * The bulk upload of a series of chapters or volumes (DEC-200). Three steps on one screen: choose the files (or a folder), review what the
 * names say (series, kind, unit, the number and the title of each file, what repeats and what is missing) and send them, three at a time.
 * Each work goes into the series with its number, unit and kind already written and locked; a number the series already has is skipped
 * and said, never doubled.
 */
export function SeriesBulk({ onBusy, onClose }) {
  const queryClient = useQueryClient();
  const openCollection = useGlobalStore((state) => state.openCollection);
  const closeModal = useGlobalStore((state) => state.closeUploadModal);
  const [rows, setRows] = useState([]);
  const [series, setSeries] = useState('');
  const [seriesTyped, setSeriesTyped] = useState(false);
  const [kind, setKind] = useState('manga');
  const [unit, setUnit] = useState('chapter');
  const [notice, setNotice] = useState('');
  const [dragging, setDragging] = useState(false);
  const [running, setRunning] = useState(false);
  const [opened, setOpened] = useState(null); // the series the last run put works in
  const stopRef = useRef(false);
  const nextId = useRef(1);
  const folder = useRef('');

  useEffect(() => { onBusy?.(running); }, [running, onBusy]);

  const started = rows.some((row) => row.status !== SENT.queued);
  const derived = useMemo(() => sortRows(deriveRows(rows, { series, unit })), [rows, series, unit]);
  const facts = useMemo(() => summarize(derived), [derived]);
  const patch = (id, change) => setRows((now) => now.map((row) => (row.id === id ? { ...row, ...change } : row)));

  const choose = (files) => {
    if (running || files.length === 0) return;
    const picked = pickSeriesFiles(files, rows.map((row) => row.file));
    const fresh = picked.kept.map((file) => ({
      id: nextId.current++, file, name: file.name, read: readNumber(file.name), typedNumber: null, typedTitle: null, status: SENT.queued, progress: 0, message: '',
    }));
    if (picked.kept.length) {
      if (!folder.current) folder.current = folderOf(picked.kept[0]);
      const all = [...rows, ...fresh];
      if (!seriesTyped) setSeries(guessSeries(all.map((row) => row.name), folder.current));
      setRows(all);
    }
    const said = [];
    if (picked.over) said.push(`Cabem ${MAX_SERIES_FILES} arquivos por vez: ${plural(picked.over, 'ficou de fora', 'ficaram de fora')}. Envie ${picked.over === 1 ? 'esse' : 'esses'} depois.`);
    if (picked.other) said.push(`${plural(picked.other, 'arquivo não é', 'arquivos não são')} CBZ, CBR nem PDF: ficaram de fora.`);
    if (picked.repeated) said.push(`${plural(picked.repeated, 'arquivo já estava', 'arquivos já estavam')} na lista.`);
    setNotice(said.join(' '));
  };

  const startOver = () => {
    if (running) return;
    setRows([]);
    setSeries('');
    setSeriesTyped(false);
    setNotice('');
    setOpened(null);
    folder.current = '';
  };

  // What goes: the rows with a number nobody else has, and, after a run, the ones that did not go through.
  const ready = (row) => (row.status === SENT.queued ? row.situation === 'new' : [SENT.error, SENT.cancelled].includes(row.status));

  const send = async (only) => {
    const list = derived.filter((row) => (only ? only.includes(row.id) : ready(row)));
    if (list.length === 0 || !series.trim() || running) return;
    stopRef.current = false;
    setRunning(true);
    setNotice('');
    list.forEach((row) => patch(row.id, { status: SENT.queued, progress: 0, message: '' }));
    const count = { sent: 0, skipped: 0, duplicates: 0, errors: 0 };
    let collectionId = opened;
    const never = await runPool(list, async (row) => {
      patch(row.id, { status: SENT.uploading, progress: 0 });
      try {
        const data = await uploadBook({
          file: row.file,
          onProgress: (progress) => patch(row.id, { progress }),
          place: { series: series.trim(), series_index: row.number, unit: row.unit, comic_kind: kind, title: row.title },
        });
        collectionId = data?.collection_id ?? collectionId;
        patch(row.id, { status: SENT.done, progress: 100 });
        count.sent += 1;
      } catch (error) {
        const message = describeUploadError(error);
        if (isInSeries(error)) {
          patch(row.id, { status: SENT.skipped, message });
          count.skipped += 1;
        } else if (error?.response?.status === 409) {
          patch(row.id, { status: SENT.duplicate, message });
          count.duplicates += 1;
        } else {
          patch(row.id, { status: SENT.error, message });
          count.errors += 1;
        }
      }
    }, SERIES_CONCURRENCY, () => stopRef.current);
    for (const row of never) patch(row.id, { status: SENT.cancelled });
    setRunning(false);
    setOpened(collectionId);
    if (count.sent > 0) refreshLibrary(queryClient);

    const issues = [];
    if (count.skipped) issues.push(`${plural(count.skipped, 'já estava na série', 'já estavam na série')} e ${count.skipped === 1 ? 'foi pulado' : 'foram pulados'}`);
    if (count.duplicates) issues.push(`${plural(count.duplicates, 'já estava', 'já estavam')} no acervo`);
    if (count.errors) issues.push(`${plural(count.errors, 'com erro', 'com erro')}`);
    if (never.length) issues.push(`${plural(never.length, 'cancelado', 'cancelados')}`);
    const head = `${plural(count.sent, 'capítulo enviado', 'capítulos enviados')}`;
    if (count.sent === 0 && issues.length === 0) return;
    if (issues.length) toast.warning(head, { message: `${issues.join(' · ')}.` });
    else toast.success(head, { message: `Serão lidos agora e logo aparecem em “${series.trim()}”.` });
  };

  const stop = () => { stopRef.current = true; };

  const left = derived.filter((row) => [SENT.error, SENT.cancelled].includes(row.status));
  const sendable = derived.filter((row) => row.status === SENT.queued && row.situation === 'new').length;
  const fix = facts.repeatedRows + facts.withoutNumber;
  const finished = started && !running;
  const included = derived.filter((row) => row.status !== SENT.queued || row.situation === 'new');
  const overall = included.length
    ? Math.round(included.reduce((sum, row) => sum + ([SENT.done, SENT.skipped, SENT.duplicate, SENT.error, SENT.cancelled].includes(row.status) ? 100 : row.status === SENT.uploading ? row.progress : 0), 0) / included.length)
    : 0;
  const doneCount = derived.filter((row) => row.status === SENT.done).length;

  const picker = (props, label) => (
    <label className={`inline-flex min-h-10 cursor-pointer items-center rounded-lg border border-border-hairline px-4 text-sm font-medium text-ink transition-colors focus-within:ring-2 focus-within:ring-brand/20 hover:bg-surface-alt ${running ? 'pointer-events-none opacity-60' : ''}`}>
      {label}
      <input
        type="file"
        multiple
        className="sr-only"
        aria-label={label}
        disabled={running}
        onChange={(event) => { choose([...event.target.files]); event.target.value = ''; }}
        {...props}
      />
    </label>
  );

  if (rows.length === 0) {
    return (
      <div className="flex flex-col gap-4">
        <div
          data-dragging={dragging}
          onDragEnter={(event) => { event.preventDefault(); setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); setDragging(true); }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setDragging(false); }}
          onDrop={async (event) => {
            event.preventDefault();
            setDragging(false);
            choose(await filesFromDrop(event.dataTransfer));
          }}
          className={`flex min-h-44 flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed px-4 py-6 text-center transition-colors duration-150 ${
            dragging ? 'border-brand bg-brand/5' : 'border-border-hairline bg-surface'
          }`}
        >
          <p className="text-sm text-ink">
            <span className="font-semibold text-brand">Escolha os capítulos ou volumes de uma série</span>
            <span className="hidden sm:inline"> ou arraste os arquivos, ou a pasta, até aqui</span>
          </p>
          <div className="flex flex-wrap justify-center gap-2">
            {picker({ accept: '.cbz,.cbr,.pdf' }, 'Escolher arquivos')}
            {picker({ webkitdirectory: '', directory: '' }, 'Escolher pasta')}
          </div>
          <p className="max-w-sm text-xs leading-relaxed text-ink-soft">
            CBZ, CBR e PDF, até {MAX_SERIES_FILES} por vez. O número de cada um é lido do nome do arquivo (Cap 1, Capítulo 02, Vol 3, #7…) e você revisa antes de enviar.
          </p>
        </div>
        {notice && <p role="alert" className="rounded-lg bg-warning-soft px-3 py-2.5 text-[13px] leading-snug text-warning">{notice}</p>}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto]">
        <label className="flex min-w-0 flex-col gap-1.5">
          <span className="font-mono text-[10px] uppercase tracking-[0.14em] text-ink-soft">Nome da série</span>
          <input
            type="text"
            value={series}
            disabled={started}
            maxLength={512}
            onChange={(event) => { setSeries(event.target.value); setSeriesTyped(true); }}
            className="min-h-10 rounded-lg border border-border-hairline bg-white px-3 text-sm text-ink outline-none focus:border-brand focus:ring-2 focus:ring-brand/15 disabled:opacity-60"
          />
        </label>
        <Choice legend="Tipo" name="series-kind" value={kind} options={KINDS} onChange={setKind} disabled={started} />
      </div>
      <Choice legend="Unidade" name="series-unit" value={unit} options={UNITS} onChange={setUnit} disabled={started} />

      <div className="flex flex-wrap gap-1.5 text-[12px]" aria-label="Resumo">
        <span className="rounded-full bg-surface-alt px-2.5 py-1 text-ink">{plural(facts.withNumber, 'com número', 'com número')}</span>
        {facts.withoutNumber > 0 && <span className="rounded-full bg-warning-soft px-2.5 py-1 text-warning">{facts.withoutNumber} sem número</span>}
        {facts.missing.length > 0 && <span className="rounded-full bg-warning-soft px-2.5 py-1 text-warning">faltam {listNumbers(facts.missing)}</span>}
        {facts.repeated.length > 0 && <span className="rounded-full bg-danger-soft px-2.5 py-1 text-danger">{plural(facts.repeated.length, 'repetido', 'repetidos')}: {listNumbers(facts.repeated)}</span>}
        {facts.first != null && <span className="rounded-full bg-surface-alt px-2.5 py-1 text-ink-soft">{facts.first === facts.last ? `número ${facts.first}` : `de ${facts.first} a ${facts.last}`}</span>}
      </div>

      <div className="max-h-[38dvh] overflow-auto rounded-xl border border-border-hairline">
        <table className="w-full min-w-[34rem] border-collapse text-left text-[13px]">
          <thead className="sticky top-0 bg-surface text-[11px] uppercase tracking-wide text-ink-soft">
            <tr>
              <th className="px-3 py-2 font-medium">Arquivo</th>
              <th className="w-20 px-2 py-2 font-medium">Número</th>
              <th className="px-2 py-2 font-medium">Fica como</th>
              <th className="px-3 py-2 font-medium">Situação</th>
              <th className="w-10" />
            </tr>
          </thead>
          <tbody>
            {derived.map((row) => {
              const line = situationOf(row);
              return (
                <tr key={row.id} data-status={row.status} data-situation={row.situation} className="border-t border-border-hairline align-middle">
                  <td className="max-w-[12rem] truncate px-3 py-1.5 text-ink" title={row.name}>{row.name}</td>
                  <td className="px-2 py-1.5">
                    <input
                      type="text"
                      inputMode="decimal"
                      aria-label={`Número de ${row.name}`}
                      value={row.typedNumber ?? (row.number == null ? '' : String(row.number))}
                      disabled={started}
                      onChange={(event) => patch(row.id, { typedNumber: event.target.value })}
                      className={`w-16 rounded-md border bg-white px-2 py-1 text-[13px] outline-none focus:ring-2 focus:ring-brand/15 disabled:bg-transparent ${
                        row.situation === 'repeated' ? 'border-danger' : 'border-border-hairline'
                      }`}
                    />
                  </td>
                  <td className="px-2 py-1.5">
                    <input
                      type="text"
                      aria-label={`Título de ${row.name}`}
                      value={row.title}
                      disabled={started}
                      maxLength={512}
                      onChange={(event) => patch(row.id, { typedTitle: event.target.value })}
                      className="w-full min-w-[9rem] rounded-md border border-border-hairline bg-white px-2 py-1 text-[13px] outline-none focus:ring-2 focus:ring-brand/15 disabled:bg-transparent"
                    />
                  </td>
                  <td className={`px-3 py-1.5 text-[12px] ${line.tone}`}>
                    {line.text}
                    {row.status === SENT.uploading && (
                      <div role="progressbar" aria-label={`Andamento de ${row.name}`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={row.progress} className="mt-1 h-1 w-full overflow-hidden rounded-full bg-surface-alt">
                        <div className="h-full rounded-full bg-brand" style={{ width: `${row.progress}%` }} />
                      </div>
                    )}
                  </td>
                  <td className="px-1 py-1.5">
                    {!started && (
                      <button
                        onClick={() => setRows((now) => now.filter((item) => item.id !== row.id))}
                        aria-label={`Tirar ${row.name}`}
                        className="flex h-8 w-8 items-center justify-center rounded-lg text-ink-faint transition-colors hover:bg-surface-alt hover:text-ink"
                      >
                        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg>
                      </button>
                    )}
                    {row.status === SENT.error && !running && (
                      <button onClick={() => send([row.id])} aria-label={`Tentar de novo ${row.name}`} className="rounded-lg px-2 py-1 text-[12px] font-semibold text-brand hover:bg-brand/10">de novo</button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {(running || started) && (
        <div className="flex flex-col gap-1.5">
          <div role="progressbar" aria-label="Andamento do envio da série" aria-valuemin={0} aria-valuemax={100} aria-valuenow={overall} className="h-2 w-full overflow-hidden rounded-full bg-surface-alt">
            <div className="h-full rounded-full bg-brand transition-[width] duration-300 ease-out" style={{ width: `${overall}%` }} />
          </div>
          <p role="status" className="font-mono text-xs text-ink-soft">
            {running ? `Enviando… ${plural(doneCount, 'enviado', 'enviados')} de ${included.length}` : `${plural(doneCount, 'capítulo enviado', 'capítulos enviados')}`}
          </p>
        </div>
      )}

      {notice && <p role="alert" className="rounded-lg bg-warning-soft px-3 py-2.5 text-[13px] leading-snug text-warning">{notice}</p>}
      {!started && !series.trim() && <p className="text-[12px] text-warning">Dê um nome à série para poder enviar.</p>}

      <div className="flex flex-wrap items-center justify-end gap-2">
        {!started && picker({ accept: '.cbz,.cbr,.pdf' }, 'Juntar mais arquivos')}
        <button onClick={finished ? onClose : startOver} disabled={running} className="min-h-10 rounded-lg px-4 py-2 text-sm font-medium text-ink-soft transition-colors hover:bg-surface-alt hover:text-ink disabled:opacity-30">
          {finished ? 'Fechar' : 'Recomeçar'}
        </button>
        {running && (
          <button onClick={stop} className="min-h-10 rounded-lg border border-border-hairline px-4 py-2 text-sm font-medium text-ink transition-colors hover:bg-surface-alt">Parar o resto</button>
        )}
        {finished && opened && (
          <button
            onClick={() => { closeModal(); openCollection(opened); }}
            className="min-h-10 rounded-lg border border-border-hairline px-4 py-2 text-sm font-medium text-ink transition-colors hover:bg-surface-alt"
          >
            Abrir a série
          </button>
        )}
        {finished && left.length > 0 && (
          <button onClick={() => send(left.map((row) => row.id))} className="min-h-10 rounded-lg bg-brand px-5 py-2 text-sm font-semibold text-white hover:brightness-110">
            {`Tentar de novo ${left.length}`}
          </button>
        )}
        {!started && (
          <button
            onClick={() => send()}
            disabled={sendable === 0 || facts.repeatedRows > 0 || !series.trim() || running}
            className="min-h-10 rounded-lg bg-brand px-5 py-2 text-sm font-semibold text-white transition-[filter,transform] duration-150 hover:brightness-110 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-40 disabled:active:scale-100"
          >
            {`Enviar ${sendable}${fix > 0 ? ` · corrigir ${fix} antes` : ''}`}
          </button>
        )}
      </div>
    </div>
  );
}
