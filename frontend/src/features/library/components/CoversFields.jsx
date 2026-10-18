import React from 'react';
import { useCollection } from '../../collections/api/useCollections';
import { endFromCount, rangeCount, suggestRange } from '../coversRange';

const inputClass = 'w-24 rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink outline-none focus:border-brand disabled:opacity-50';

/**
 * What the file of a work contains of the story (DEC-202): from which chapter to which. A volume says the chapters it collected, "Cap 12-13" the
 * two it holds, a complete edition all. Optional; the series reads by it (what is read, what is missing, where to go on). The quantity can be
 * given instead of the last chapter, and the chapters that already say "volume N" suggest the range of the volume.
 */
export function CoversFields({ from, to, onFrom, onTo, unit, position, collectionId, disabled }) {
  const [quantity, setQuantity] = React.useState('');
  const { data } = useCollection(unit === 'volume' && collectionId ? collectionId : null);
  const suggestion = unit === 'volume' ? suggestRange(data?.works, position ? Number(position) : null) : null;
  const count = rangeCount(from, to);
  const differs = suggestion && (Number(from) !== suggestion.from || Number(to) !== suggestion.to);
  return (
    <fieldset aria-label="O que este arquivo contém" className="flex flex-col gap-2 sm:col-span-2">
      <legend className="mb-1 text-xs font-medium text-ink-soft">
        O que este arquivo contém <span className="font-normal text-ink-faint">(opcional)</span>
      </legend>
      <div className="flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-soft">
          Do capítulo
          <input className={inputClass} inputMode="decimal" value={from} onChange={(e) => onFrom(e.target.value)} disabled={disabled} />
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-soft">
          Ao capítulo
          <input className={inputClass} inputMode="decimal" value={to} onChange={(e) => onTo(e.target.value)} disabled={disabled} />
        </label>
        <span className="pb-2 text-xs text-ink-soft">{count != null ? `= ${count} ${count === 1 ? 'capítulo' : 'capítulos'}` : ''}</span>
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-soft">
          ou só a quantidade
          <input
            className={inputClass}
            inputMode="numeric"
            aria-label="Quantidade de capítulos"
            placeholder="Ex.: 9"
            value={quantity}
            onChange={(e) => {
              setQuantity(e.target.value);
              const end = endFromCount(from, e.target.value);
              if (end) onTo(end);
            }}
            disabled={disabled || !from}
            title={from ? 'Com o primeiro capítulo dito, a quantidade dá o último' : 'Diga primeiro o capítulo em que começa'}
          />
        </label>
      </div>
      {differs && (
        <div className="rounded-lg border border-dashed border-brand/50 bg-brand/5 px-3 py-2 text-xs text-ink-soft">
          {suggestion.chapters} {suggestion.chapters === 1 ? 'obra da série já diz' : 'obras da série já dizem'} “volume {position}”: capítulos {suggestion.from} a {suggestion.to}.{' '}
          <button
            type="button"
            onClick={() => { onFrom(String(suggestion.from)); onTo(String(suggestion.to)); }}
            disabled={disabled}
            className="font-semibold text-brand underline decoration-dotted underline-offset-4"
          >
            Usar {suggestion.from} a {suggestion.to}
          </button>
        </div>
      )}
    </fieldset>
  );
}
