import React, { useEffect, useState } from 'react';
import { Skeleton } from '../../../components/ui/Skeleton';
import { useDictionaryLookup } from '../api/useDictionaryLookup';
import { LOOKUP_LANGUAGES, posLabel, senseFormOf, tagsLine, visibleSenses } from '../dictionaryLookup';

function Senses({ data }) {
  const [open, setOpen] = useState(false);
  const { shown, more, all } = visibleSenses(data);
  const list = open ? all : shown;
  return (
    <>
      <ol className="mt-1.5 flex list-decimal flex-col gap-1.5 pl-5 text-[14px] leading-snug text-ink">
        {list.map((sense, i) => {
          const of = senseFormOf(sense);
          return (
            <li key={i}>
              {tagsLine(sense.tags).length > 0 && <span className="mr-1.5 text-[11px] italic text-ink-faint">{tagsLine(sense.tags).join(', ')}</span>}
              {sense.glosses?.join('; ') || (of.length > 0 && `forma de ${of.join(', ')}`)}
              {sense.examples?.[0] && (
                <span className="mt-0.5 block text-[12px] italic text-ink-soft">
                  “{sense.examples[0].text}”{sense.examples[0].translation ? ` — ${sense.examples[0].translation}` : ''}
                </span>
              )}
            </li>
          );
        })}
      </ol>
      {more > 0 && (
        <button onClick={() => setOpen((o) => !o)} className="mt-1 min-h-9 text-[12px] text-brand hover:underline">
          {open ? 'Mostrar menos' : `Mostrar mais ${more} ${more === 1 ? 'sentido' : 'sentidos'}`}
        </button>
      )}
    </>
  );
}

function Entry({ item }) {
  const { entry } = item;
  const data = typeof entry.data === 'string' ? JSON.parse(entry.data) : entry.data;
  return (
    <article className="border-t border-border-hairline py-3 first:border-t-0 first:pt-0">
      {item.kind === 'lemma' && (
        <p className="mb-1 text-[12px] text-ink-soft">
          {item.form ? <>“{item.form}” vem de</> : 'Vem de'}
          {tagsLine(item.tags).length > 0 && <span className="ml-1 text-ink-faint">({tagsLine(item.tags).join(', ')})</span>}
        </p>
      )}
      {item.kind === 'translation' && (
        <p className="mb-1 text-[12px] text-ink-soft">
          “{item.via}” está listado como tradução de
          {item.sense && <span className="ml-1 text-ink-faint">({item.sense})</span>}
        </p>
      )}
      <h3 className="flex flex-wrap items-baseline gap-2">
        <span className="font-display text-xl font-semibold text-ink">{entry.word}</span>
        {entry.pos && <span className="text-[12px] italic text-ink-soft">{posLabel(entry.pos)}</span>}
        {data?.ipa && <span className="font-mono text-[11px] text-ink-faint">{data.ipa}</span>}
      </h3>
      <Senses data={data} />
    </article>
  );
}

/**
 * The card of a word the reader selected (#109): what the dictionary says of it, in the language of the file (which the
 * reader may change), with where it comes from. A word that is a form of another shows the word it comes from; a word of
 * another language than the dictionary's is found by the translations the entries list.
 */
export function DictionaryCard({ word, language, onClose }) {
  const [lang, setLang] = useState(language);
  useEffect(() => setLang(language), [language, word]);
  const { data, isLoading, isError } = useDictionaryLookup({ word, lang });

  useEffect(() => {
    const onKey = (event) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const languageName = LOOKUP_LANGUAGES.find(([c]) => c === lang)?.[1] ?? lang;
  return (
    <aside
      role="dialog"
      aria-label={`Dicionário: ${word}`}
      className="fixed inset-x-0 bottom-0 z-[70] mx-auto flex max-h-[70dvh] w-full max-w-lg animate-rise-in flex-col rounded-t-2xl border border-border-hairline bg-white shadow-2xl sm:bottom-4 sm:right-4 sm:left-auto sm:mx-0 sm:rounded-2xl"
    >
      <header className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline px-4 py-3">
        <div className="min-w-0">
          <p className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Dicionário</p>
          <p className="truncate font-display text-lg font-semibold text-ink">{word}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <select
            value={lang}
            onChange={(event) => setLang(event.target.value)}
            aria-label="Idioma da palavra"
            className="min-h-11 rounded-lg border border-border-hairline bg-white px-2 text-[13px] text-ink"
          >
            {LOOKUP_LANGUAGES.map(([code, name]) => <option key={code} value={code}>{name}</option>)}
          </select>
          <button onClick={onClose} aria-label="Fechar o dicionário" className="flex min-h-11 min-w-11 items-center justify-center rounded-lg bg-surface-alt text-lg text-ink-soft hover:bg-border-hairline hover:text-ink">
            ✕
          </button>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
        {isLoading && <Skeleton label="Procurando a palavra" className="h-24 w-full" />}
        {isError && <p role="alert" className="text-sm text-danger">Não foi possível consultar o dicionário.</p>}
        {data && !data.installed && (
          <p className="text-sm text-ink-soft">
            Nenhum dicionário está instalado neste servidor. O dono do acervo pode instalar um em Administração → Dicionários.
          </p>
        )}
        {data?.installed && data.items.length === 0 && (
          <p className="text-sm text-ink-soft">
            Não achei “{word}” em {languageName}. Se a palavra é de outro idioma, troque o idioma acima; se está flexionada ou com grafia
            diferente, tente selecionar só a palavra.
          </p>
        )}
        {data?.items.map((item) => <Entry key={`${item.kind}-${item.entry.id}`} item={item} />)}
      </div>

      {data?.sources.length > 0 && (
        <footer className="shrink-0 border-t border-border-hairline px-4 py-2 text-[11px] text-ink-faint">
          Fonte: {data.sources.map((s, i) => (
            <span key={s.package}>
              {i > 0 && '; '}
              <a href={s.sourceUrl} target="_blank" rel="noopener noreferrer" className="underline">{s.name}</a>
              {' ('}<a href={s.licenseUrl} target="_blank" rel="noopener noreferrer" className="underline">{s.license}</a>{')'}
            </span>
          ))}
        </footer>
      )}
    </aside>
  );
}
