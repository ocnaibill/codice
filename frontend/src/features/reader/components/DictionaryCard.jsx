import React, { useEffect, useState } from 'react';
import { Skeleton } from '../../../components/ui/Skeleton';
import { useDictionaryLookup } from '../api/useDictionaryLookup';
import { LOOKUP_LANGUAGES, groupTranslations, languageName, posLabel, senseFormOf, tagsLine, visibleSenses } from '../dictionaryLookup';
import { getDictionaryTarget, saveDictionaryTarget } from '../preferences';

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

function Translations({ data, prefer }) {
  const groups = groupTranslations(data?.translations, prefer);
  if (groups.length === 0) return null;
  return (
    <p className="mt-2 text-[13px] leading-snug text-ink-soft">
      <span className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Tradução</span>
      {groups.map((g) => (
        <span key={g.lang} className="mt-0.5 block">
          <span className="text-ink-faint">{g.name}: </span>
          <span className="text-ink">{g.words.join(', ')}</span>
          {g.more > 0 && <span className="text-ink-faint"> e mais {g.more}</span>}
        </span>
      ))}
    </p>
  );
}

/**
 * What the lookup tried through English when the dictionaries do not link the word's language to the one the reader wants
 * (plan B). Always said as what it is: by way of English, an approximation with candidates, never as the answer.
 */
function Bridge({ bridge, word }) {
  const candidates = bridge.candidates ?? [];
  return (
    <section aria-label="Via inglês" className="mt-3 rounded-xl border border-border-hairline bg-surface-alt px-3 py-2.5">
      <h2 className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Via inglês</h2>
      {candidates.length > 0 ? (
        <>
          <p className="mt-1 text-[12px] leading-snug text-ink-soft">
            Os dicionários instalados não ligam “{word}” a {languageName(bridge.to)} diretamente. Pela ponte do inglês, estes são
            candidatos: uma aproximação, que pode não ser o sentido da palavra aqui.
          </p>
          <ul className="mt-1.5 flex flex-col gap-1">
            {candidates.map((c) => (
              <li key={c.english} className="text-[14px] leading-snug text-ink">
                <span className="font-display font-semibold">{c.english}</span>
                <span className="text-ink-faint"> → </span>
                {c.words.join(', ')}
              </li>
            ))}
          </ul>
        </>
      ) : (
        <p className="mt-1 text-[12px] leading-snug text-ink-soft">
          Nem pela ponte do inglês achei “{word}” em {languageName(bridge.to)}.
        </p>
      )}
      {!bridge.available && (
        <p className="mt-1.5 text-[12px] leading-snug text-ink-soft">
          O dicionário de inglês não está instalado: com ele a ponte acha mais. O dono do acervo o instala em Administração → Dicionários.
        </p>
      )}
    </section>
  );
}

function Entry({ item, prefer }) {
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
      <Translations data={data} prefer={prefer} />
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
  const [target, setTarget] = useState(getDictionaryTarget);
  const chooseTarget = (code) => {
    setTarget(code);
    saveDictionaryTarget(code);
  };
  const { data, isLoading, isError } = useDictionaryLookup({ word, lang, prefer: target });

  useEffect(() => {
    const onKey = (event) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  // With the words of more than one package, each is said under the name of the package it comes from.
  const items = data?.items ?? [];
  const sourceName = (id) => data?.sources.find((s) => s.package === id)?.name ?? id;
  const groups = [];
  for (const item of items) {
    const last = groups[groups.length - 1];
    if (last && last.package === item.entry.package) last.items.push(item);
    else groups.push({ package: item.entry.package, items: [item] });
  }
  const showGroups = new Set(items.map((i) => i.entry.package)).size > 1;
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

      <label className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline px-4 py-1.5 text-[12px] text-ink-soft">
        Definições primeiro em
        <select
          value={target}
          onChange={(event) => chooseTarget(event.target.value)}
          className="min-h-11 rounded-lg border border-border-hairline bg-white px-2 text-[13px] text-ink"
        >
          {LOOKUP_LANGUAGES.map(([code, name]) => <option key={code} value={code}>{name}</option>)}
        </select>
      </label>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
        {isLoading && <Skeleton label="Procurando a palavra" className="h-24 w-full" />}
        {isError && <p role="alert" className="text-sm text-danger">Não foi possível consultar o dicionário.</p>}
        {data && !data.installed && (
          <p className="text-sm text-ink-soft">
            Nenhum dicionário está instalado neste servidor. O dono do acervo pode instalar um em Administração → Dicionários.
          </p>
        )}
        {data?.installed && data.items.length === 0 && !(data.bridge?.candidates?.length > 0) && (
          <p className="text-sm text-ink-soft">
            Não achei “{word}” em {languageName(lang)}. Se a palavra é de outro idioma, troque o idioma acima; se está flexionada ou com grafia
            diferente, tente selecionar só a palavra.
          </p>
        )}
        {groups.map((group) => (
          <section key={group.package}>
            {showGroups && <h2 className="mb-1 mt-3 font-mono text-[10px] uppercase tracking-widest text-ink-faint first:mt-0">{sourceName(group.package)}</h2>}
            {group.items.map((item) => <Entry key={`${item.kind}-${item.entry.id}`} item={item} prefer={target} />)}
          </section>
        ))}
        {data?.bridge && <Bridge bridge={data.bridge} word={word} />}
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
