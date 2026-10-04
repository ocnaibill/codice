import { useState } from 'react';
import { describeError, useLoginEvents, useSetLoginRetention } from '../api/admin';
import { LoadError } from '../../../components/ui/LoadError';
import { Notice } from '../../../components/ui/Notice';
import { loginDetail, loginWhen, loginWho, RESULTS, RESULT_FILTERS } from '../loginText';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';

const TONE = { ok: 'text-success', bad: 'text-danger', warn: 'text-warning', neutral: 'text-ink-soft' };
const DAY = 24 * 60 * 60 * 1000;
const PERIODS = [['', 'Todo o período'], ['1', 'Últimas 24 horas'], ['7', 'Últimos 7 dias'], ['30', 'Últimos 30 dias']];
const MIN_DAYS = 7;
const MAX_DAYS = 3650;

/** The "from" of a period in days, as a date the server reads; nothing for "all". */
const sinceDays = (days) => (days ? new Date(Date.now() - Number(days) * DAY).toISOString() : '');

function Retention({ days, isOwner }) {
  const save = useSetLoginRetention();
  const [value, setValue] = useState(String(days));
  const [editing, setEditing] = useState(false);
  const number = Number(value);
  const valid = Number.isInteger(number) && number >= MIN_DAYS && number <= MAX_DAYS;

  if (!isOwner) {
    return <p className="text-[12px] text-ink-faint">Este registro é guardado por {days} dias e depois apagado.</p>;
  }
  if (!editing) {
    return (
      <p className="text-[12px] text-ink-faint">
        Guardado por <strong className="text-ink-soft">{days} dias</strong> e depois apagado. {' '}
        <button type="button" onClick={() => { setValue(String(days)); setEditing(true); }} className="text-brand underline-offset-2 hover:underline">Mudar</button>
      </p>
    );
  }
  return (
    <form
      onSubmit={(event) => { event.preventDefault(); if (valid) save.mutate(number, { onSuccess: () => setEditing(false) }); }}
      className="flex flex-wrap items-end gap-2"
    >
      <label className="flex flex-col gap-1 text-[12px] text-ink-soft">
        Guardar por quantos dias ({MIN_DAYS} a {MAX_DAYS})
        <input
          type="number" min={MIN_DAYS} max={MAX_DAYS} value={value} aria-label="Dias de guarda"
          onChange={(event) => setValue(event.target.value)}
          className="w-28 rounded bg-surface px-3 py-2 text-[14px] text-ink outline-none"
        />
      </label>
      <Btn tone="primary" type="submit" disabled={!valid || save.isPending}>{save.isPending ? 'Guardando…' : 'Guardar'}</Btn>
      <Btn type="button" onClick={() => setEditing(false)}>Cancelar</Btn>
      {!valid && value !== '' && <p role="alert" className="w-full text-[12px] text-danger">Use um número inteiro de {MIN_DAYS} a {MAX_DAYS}.</p>}
      <ErrorNote>{save.isError ? describeError(save.error) : ''}</ErrorNote>
    </form>
  );
}

function Row({ entry }) {
  const result = RESULTS[entry.result] ?? { label: entry.result, tone: 'neutral' };
  const when = loginWhen(entry);
  return (
    <li className="flex flex-wrap items-start justify-between gap-x-4 gap-y-1 py-3">
      <div className="min-w-0">
        <p className="text-[14px] text-ink">
          <span className="font-medium">{loginWho(entry)}</span>
          <span className={`ml-2 text-[12px] font-medium ${TONE[result.tone]}`}>{result.label}</span>
        </p>
        <p className="text-[12px] text-ink-soft">{loginDetail(entry)}</p>
      </div>
      <p className="shrink-0 text-[12px] text-ink-faint" title={when.title}>{when.text}</p>
    </li>
  );
}

/**
 * "Entradas" (DEC-121, #137): who got in, who tried and failed, from where and by which way, for the owner and the
 * administrators. A run of the same failure is one line with how many times; the address is the one the server believes
 * (CODICE_TRUSTED_PROXIES); and the names typed for accounts that do not exist are the owner's alone, because
 * a field for a name is where a password lands when someone types in the wrong one.
 */
export function LoginsTab({ isOwner }) {
  const [result, setResult] = useState('');
  const [period, setPeriod] = useState('');
  const [typedName, setTypedName] = useState('');
  const [username, setUsername] = useState('');
  const filters = { result, username, from: sinceDays(period) };
  const { data, isLoading, isError, error, refetch, isRefetching, fetchNextPage, hasNextPage, isFetchingNextPage } = useLoginEvents(filters);
  const entries = data?.pages.flatMap((page) => page.entries) ?? [];
  const first = data?.pages[0];
  const filtered = Boolean(result || period || username);

  return (
    <Section
      title="Entradas"
      hint="Quem entrou, quem tentou e não conseguiu, de onde e por qual caminho. Uma série da mesma falha vira uma linha só, com quantas vezes."
    >
      {first?.sameAddress?.warn && (
        <Notice tone="warning" className="mb-4" title="Todas as entradas vêm do mesmo endereço">
          Nos últimos 7 dias, tudo o que está aqui veio de <strong>{first.sameAddress.address}</strong>, que é um endereço de dentro da rede. Isso costuma
          ser um proxy ou túnel na frente do Códice sem que o servidor saiba: ele não está enxergando quem entra de verdade, e o limite de tentativas
          de login vale para todos juntos. Defina <code>CODICE_TRUSTED_PROXIES</code> com o endereço do proxy (o README explica).
        </Notice>
      )}

      <div className="mb-3 flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1 text-[12px] text-ink-soft">
          O que aconteceu
          <select value={result} onChange={(event) => setResult(event.target.value)} aria-label="Filtrar pelo resultado" className="rounded bg-surface px-3 py-2 text-[13px] text-ink outline-none">
            <option value="">Tudo</option>
            {RESULT_FILTERS.map((key) => <option key={key} value={key}>{RESULTS[key].label}</option>)}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-[12px] text-ink-soft">
          Quando
          <select value={period} onChange={(event) => setPeriod(event.target.value)} aria-label="Filtrar pelo período" className="rounded bg-surface px-3 py-2 text-[13px] text-ink outline-none">
            {PERIODS.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <form onSubmit={(event) => { event.preventDefault(); setUsername(typedName.trim()); }} className="flex items-end gap-2">
          <label className="flex flex-col gap-1 text-[12px] text-ink-soft">
            Conta
            <input value={typedName} onChange={(event) => setTypedName(event.target.value)} placeholder="nome de usuário" aria-label="Filtrar pela conta" className="w-40 rounded bg-surface px-3 py-2 text-[13px] text-ink outline-none" />
          </label>
          <Btn type="submit">Buscar</Btn>
        </form>
        {filtered && <Btn onClick={() => { setResult(''); setPeriod(''); setTypedName(''); setUsername(''); }}>Limpar filtros</Btn>}
      </div>

      {isLoading && <Loading />}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as entradas.</LoadError>}
      {data && entries.length === 0 && (
        <Empty>{filtered ? 'Nenhuma entrada com esses filtros.' : 'Ainda não há entradas registradas.'}</Empty>
      )}
      <ul className="divide-y divide-border-hairline">
        {entries.map((entry) => <Row key={entry.id} entry={entry} />)}
      </ul>
      {hasNextPage && (
        <div className="mt-3">
          <Btn onClick={() => fetchNextPage()} disabled={isFetchingNextPage}>{isFetchingNextPage ? 'Carregando…' : 'Mostrar mais'}</Btn>
        </div>
      )}
      {first && <div className="mt-5 border-t border-border-hairline pt-3"><Retention days={first.retentionDays} isOwner={isOwner} /></div>}
      {!isOwner && first && <p className="mt-1 text-[12px] text-ink-faint">Os nomes digitados por quem não tem conta só o dono vê.</p>}
    </Section>
  );
}
