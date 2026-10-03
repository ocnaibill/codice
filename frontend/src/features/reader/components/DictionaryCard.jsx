import React, { useEffect, useState } from 'react';
import { Skeleton } from '../../../components/ui/Skeleton';
import { useDictionaryLookup } from '../api/useDictionaryLookup';
import { useDictionaryLanguages } from '../api/useDictionaryLanguages';
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
function Bridge({ bridge, word, first = false }) {
  const candidates = bridge.candidates ?? [];
  return (
    <section aria-label="Via inglês" className={`${first ? 'mb-3' : 'mt-3'} rounded-xl border border-border-hairline bg-surface-alt px-3 py-2.5`}>
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
  // What the installed dictionaries can be asked in: only that is offered. Until it is known the lookup waits (it would be
  // asked twice otherwise, once with the language remembered and once with the one that can be had); if it cannot be known,
  // every language is offered, as before.
  const installed = useDictionaryLanguages();
  const known = installed.data ?? null;
  const settled = !installed.isLoading;
  const wordLanguages = known ? LOOKUP_LANGUAGES.filter(([c]) => known.words.includes(c)) : LOOKUP_LANGUAGES;
  const definitionLanguages = known ? LOOKUP_LANGUAGES.filter(([c]) => known.definitions.includes(c)) : LOOKUP_LANGUAGES;
  // The language the definitions come first in: the one remembered, when there is a dictionary in it; else the one of the
  // word, or the first there is. The one remembered is not lost: it is there again when a dictionary in it is installed.
  const definitionCodes = definitionLanguages.map(([c]) => c);
  const wanted = !known || definitionCodes.includes(target) ? target : (definitionCodes.includes(lang) ? lang : (definitionCodes[0] ?? target));
  const wordAvailable = !known || known.words.includes(lang);
  const { data, isLoading, isError } = useDictionaryLookup({ word, lang, prefer: wanted, enabled: settled });

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
  // What the bridge found is what the person is after when the dictionaries do not link the two languages, and the English
  // entries that come before it are only its first step: it is shown first, and the entries after.
  const bridge = wordAvailable ? data?.bridge : null; // with no dictionary of the language there is nothing to bridge from
  const bridgeFirst = (bridge?.candidates?.length ?? 0) > 0;
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
            {wordLanguages.map(([code, name]) => <option key={code} value={code}>{name}</option>)}
            {/* the language of the word when there is no dictionary in it: shown as the one chosen, and not offered */}
            {!wordAvailable && <option value={lang} disabled hidden>{languageName(lang)}</option>}
          </select>
          <button onClick={onClose} aria-label="Fechar o dicionário" className="flex min-h-11 min-w-11 items-center justify-center rounded-lg bg-surface-alt text-lg text-ink-soft hover:bg-border-hairline hover:text-ink">
            ✕
          </button>
        </div>
      </header>

      {/* with one language to choose there is nothing to choose */}
      {(!known || definitionLanguages.length > 1) && (
        <label className="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline px-4 py-1.5 text-[12px] text-ink-soft">
          Definições primeiro em
          <select
            value={wanted}
            onChange={(event) => chooseTarget(event.target.value)}
            className="min-h-11 rounded-lg border border-border-hairline bg-white px-2 text-[13px] text-ink"
          >
            {definitionLanguages.map(([code, name]) => <option key={code} value={code}>{name}</option>)}
          </select>
        </label>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
        {(isLoading || !settled) && <Skeleton label="Procurando a palavra" className="h-24 w-full" />}
        {isError && <p role="alert" className="text-sm text-danger">Não foi possível consultar o dicionário.</p>}
        {data && !data.installed && (
          <p className="text-sm text-ink-soft">
            Nenhum dicionário está instalado neste servidor. O dono do acervo pode instalar um em Administração → Dicionários.
          </p>
        )}
        {data?.installed && !wordAvailable && (
          <p className="text-sm text-ink-soft">
            Nenhum dicionário instalado neste servidor tem palavras de {languageName(lang)}. O dono do acervo pode instalar um em
            Administração → Dicionários.
          </p>
        )}
        {data?.installed && wordAvailable && data.items.length === 0 && !bridgeFirst && (
          <p className="text-sm text-ink-soft">
            Não achei “{word}” em {languageName(lang)}. Se a palavra é de outro idioma, troque o idioma acima; se está flexionada ou com grafia
            diferente, tente selecionar só a palavra.
          </p>
        )}
        {bridgeFirst && <Bridge bridge={bridge} word={word} first />}
        {groups.map((group) => (
          <section key={group.package}>
            {showGroups && <h2 className="mb-1 mt-3 font-mono text-[10px] uppercase tracking-widest text-ink-faint first:mt-0">{sourceName(group.package)}</h2>}
            {group.items.map((item) => <Entry key={`${item.kind}-${item.entry.id}`} item={item} prefer={target} />)}
          </section>
        ))}
        {bridge && !bridgeFirst && <Bridge bridge={bridge} word={word} />}
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
