import { useEffect, useState } from 'react';
import { useNames, useDivideNames, describeError } from '../api/admin';
import { familyFirst, initialPlaces, partsOf, wordsOf } from '../nameParts';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';

const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

function Row({ person, places, onPlaces, dealtWith, busy, onSave }) {
  const words = wordsOf(person.name);
  const parts = partsOf(person.name, places);
  const toggle = (index) => onPlaces(places.includes(index) ? places.filter((p) => p !== index) : [...places, index]);
  const unchanged = dealtWith && partsOf(person.name, initialPlaces(person)).family === parts.family;

  return (
    <li className="py-4">
      <p className="text-[14px] font-medium text-ink">{person.name}</p>
      <p className="text-[12px] text-ink-faint">
        {count(person.works, 'obra', 'obras')}
        {person.titles?.length > 0 && `: ${person.titles.join(', ')}${person.works > person.titles.length ? '…' : ''}`}
      </p>
      <div role="group" aria-label={`Palavras do sobrenome de ${person.name}`} className="mt-2 flex flex-wrap gap-2">
        {words.map((word, index) => (
          <button
            // The words of a name can repeat ("Ana Ana Silva"), so the place is what tells them apart.
            key={index}
            type="button"
            aria-pressed={places.includes(index)}
            onClick={() => toggle(index)}
            className={`rounded-full px-3 py-1 text-[13px] ${places.includes(index) ? 'bg-brand text-white' : 'bg-surface-alt text-ink'}`}
          >
            {word}
          </button>
        ))}
      </div>
      <p className="mt-2 text-[12px] text-ink-soft">
        {person.undivided && dealtWith && places.length === 0
          ? 'Sem sobrenome: aparece sempre como está escrito.'
          : places.length === 0
            ? 'Toque nas palavras do sobrenome.'
            : `Sobrenome primeiro: ${familyFirst(parts)}`}
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        <Btn tone="primary" onClick={() => onSave({ id: person.id, ...parts })} disabled={busy || places.length === 0 || unchanged}>
          {dealtWith ? 'Salvar' : 'Confirmar'}
        </Btn>
        {!(dealtWith && person.undivided) && (
          <Btn onClick={() => onSave({ id: person.id, undivided: true })} disabled={busy}>Sem sobrenome</Btn>
        )}
        {dealtWith && <Btn onClick={() => onSave({ id: person.id, family: '', given: '' })} disabled={busy}>Voltar para a lista</Btn>}
      </div>
    </li>
  );
}

/**
 * The surnames of the people of the library. To show a name as "Sobrenome, Nome" (and to sort by it) the system has to know
 * which words are the surname, and that depends on the culture ("Gabriel García Márquez", "Ursula K. Le Guin"), so it only proposes
 * and someone from the staff confirms, picking the words if the proposal is not right (DEC-094, DEC-139). Whoever was dealt with
 * can be changed, or taken back to the list.
 */
export function PersonNames() {
  const [dealtWith, setDealtWith] = useState(false);
  const [page, setPage] = useState(1);
  const [typed, setTyped] = useState('');
  const [q, setQ] = useState('');
  const [choices, setChoices] = useState({}); // person id -> the places of the surname the person chose, for the ones that were touched
  const [progress, setProgress] = useState(0);
  const [total, setTotal] = useState(0); // how many the running save has to do
  const [confirming, setConfirming] = useState(null); // the rows waiting for a yes to "confirm the proposals of this page"
  const [done, setDone] = useState('');
  const { data, isLoading, isError, error, refetch, isRefetching } = useNames({ state: dealtWith ? 'done' : undefined, q, page });
  const divide = useDivideNames();
  const rows = data?.data ?? [];
  const totalPages = data?.totalPages ?? 1;
  const placesOf = (person) => choices[person.id] ?? initialPlaces(person);
  // The last page has none left (its people were dealt with): back to the last one there is.
  const gone = !!data && rows.length === 0 && page > 1;
  useEffect(() => {
    if (gone) setPage(Math.max(1, totalPages));
  }, [gone, totalPages]);

  const run = (items, said) => {
    setDone('');
    setProgress(0);
    setTotal(items.length);
    divide.mutate({ items, onProgress: setProgress }, { onSuccess: () => setDone(said) });
  };
  const save = (person) => (change) => {
    const said = change.undivided
      ? `“${person.name}” fica sem sobrenome: aparece sempre como está escrito.`
      : change.family === ''
        ? `“${person.name}” voltou para a lista dos nomes a dividir.`
        : `“${person.name}” aparece como “${familyFirst(change)}” na ordem sobrenome primeiro.`;
    run([change], said);
  };
  const proposals = rows.filter((person) => placesOf(person).length > 0).map((person) => ({ id: person.id, ...partsOf(person.name, placesOf(person)) }));
  const switchTo = (next) => {
    setDealtWith(next);
    setPage(1);
    setDone('');
  };

  return (
    <Section
      title="Sobrenomes dos autores"
      hint="Para mostrar um nome como “Sobrenome, Nome” e ordenar por ele, o Códice precisa saber qual palavra é o sobrenome, e isso depende da cultura. O sistema só sugere: toque nas palavras do sobrenome e confirme. Os com mais obras vêm primeiro."
    >
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div role="group" aria-label="Quais nomes ver" className="flex gap-2">
          <Btn tone={dealtWith ? 'neutral' : 'primary'} aria-pressed={!dealtWith} onClick={() => switchTo(false)}>A dividir</Btn>
          <Btn tone={dealtWith ? 'primary' : 'neutral'} aria-pressed={dealtWith} onClick={() => switchTo(true)}>Já resolvidos</Btn>
        </div>
        <form
          className="flex flex-1 gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            setQ(typed.trim());
            setPage(1);
          }}
        >
          <input
            value={typed}
            onChange={(event) => setTyped(event.target.value)}
            placeholder="Buscar um nome"
            aria-label="Buscar um nome"
            className="min-w-[160px] flex-1 rounded bg-surface px-3 py-1.5 text-[13px] outline-none"
          />
          <Btn type="submit">Buscar</Btn>
        </form>
      </div>
      {data && (
        <p className="mb-2 text-[13px] text-ink-soft">
          {data.total === 0
            ? dealtWith ? 'Nenhum nome resolvido ainda.' : q ? 'Nenhum nome a dividir com essa busca.' : 'Nenhum nome a dividir.'
            : dealtWith ? `${count(data.total, 'pessoa resolvida', 'pessoas resolvidas')}.` : `${count(data.total, 'nome a dividir', 'nomes a dividir')}.`}
        </p>
      )}
      {isLoading && <Loading />}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar os nomes.</LoadError>}
      {!dealtWith && proposals.length > 0 && (
        <div className="mb-2">
          <Btn onClick={() => setConfirming(proposals)} disabled={divide.isPending}>
            Confirmar {proposals.length === 1 ? 'a sugestão' : `as ${proposals.length} sugestões`} desta página
          </Btn>
        </div>
      )}
      <ul className="divide-y divide-border-hairline" aria-label={dealtWith ? 'Nomes resolvidos' : 'Nomes a dividir'}>
        {rows.map((person) => (
          <Row
            key={person.id}
            person={person}
            places={placesOf(person)}
            onPlaces={(next) => setChoices((all) => ({ ...all, [person.id]: next }))}
            dealtWith={dealtWith}
            busy={divide.isPending}
            onSave={save(person)}
          />
        ))}
      </ul>
      {totalPages > 1 && (
        <nav className="mt-3 flex items-center gap-3 text-[12px] text-ink-soft" aria-label="Páginas dos nomes">
          <Btn onClick={() => setPage(page - 1)} disabled={page <= 1}>Anterior</Btn>
          <span>Página {page} de {totalPages}</span>
          <Btn onClick={() => setPage(page + 1)} disabled={page >= totalPages}>Próxima</Btn>
        </nav>
      )}
      {divide.isPending && <p role="status" className="mt-2 text-[13px] text-ink-soft">Salvando… {progress} de {total}.</p>}
      {done && <p role="status" className="mt-2 text-[13px] text-ink-soft">{done}</p>}
      <ErrorNote>{divide.isError && describeError(divide.error)}</ErrorNote>

      {confirming && (
        <ConfirmDialog
          title="Confirmar as sugestões desta página?"
          message={
            <p>
              {count(confirming.length, 'pessoa passa', 'pessoas passam')} a ter o sobrenome que está marcado no nome dela, como nos botões desta página.
              Nada é renomeado, e dá para corrigir em “Já resolvidos”.
            </p>
          }
          choices={[{ label: 'Confirmar', value: true, tone: 'primary' }]}
          onChoose={() => {
            const items = confirming;
            setConfirming(null);
            run(items, `${count(items.length, 'pessoa tem', 'pessoas têm')} agora o sobrenome confirmado.`);
          }}
          onCancel={() => setConfirming(null)}
        />
      )}
    </Section>
  );
}
