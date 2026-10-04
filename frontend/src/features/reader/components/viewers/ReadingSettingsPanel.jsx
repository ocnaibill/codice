import React, { useRef } from 'react';
import { useDialog } from '../../../../lib/useDialog';
import { READING_THEMES, READING_FONTS, READING_MARGINS, READING_SPACING, SIZE_MIN, SIZE_MAX, SIZE_STEP } from '../../epubThemes';

const stepButton =
  'flex h-11 min-w-11 items-center justify-center rounded-full border border-border-hairline bg-white text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 disabled:opacity-30 disabled:hover:bg-white disabled:active:scale-100';

const Group = ({ label, children }) => (
  <div role="group" aria-label={label} className="flex flex-col gap-2">
    <span className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">{label}</span>
    {children}
  </div>
);

const FONT_STYLE = {
  'sem-serifa': { fontFamily: '"Plus Jakarta Sans Variable", system-ui, sans-serif' },
  serifada: { fontFamily: '"Newsreader Variable", Georgia, serif' },
  dislexia: { fontFamily: '"OpenDyslexic", system-ui, sans-serif' },
};

/**
 * How the text looks, for the EPUB and the text readers alike: the size, the color of the page, the font, the space
 * between lines, the margins and whether the lines are justified. Every change is made at once. `bookLabel` is the name of "what the file has" (the book's own).
 */
export default function ReadingSettingsPanel({ settings, onChange, onClose, bookLabel = 'Do livro' }) {
  // Opened from a button of the controls: the focus goes in, Escape closes it and the focus goes back to that button. Tab is free.
  const ref = useRef(null);
  useDialog(ref, { onEscape: onClose, trap: false });
  const { theme, font, size, spacing, margins, justify } = settings;
  const choice = (selected) =>
    `flex min-h-11 flex-1 items-center justify-center rounded-lg border px-3 text-sm transition-[background-color,border-color] duration-150 ${
      selected ? 'border-brand bg-brand/10 font-medium text-brand' : 'border-border-hairline bg-white text-ink-soft hover:bg-surface-alt hover:text-ink'
    }`;

  return (
    <div ref={ref} role="dialog" aria-label="Aparência do texto" className="flex flex-col gap-4">
      <Group label="Tamanho">
        <div className="flex items-center justify-between gap-3">
          <button onClick={() => onChange({ size: size - SIZE_STEP })} disabled={size <= SIZE_MIN} aria-label="Diminuir a letra" className={stepButton}>
            <span aria-hidden="true" className="text-sm">A</span>
          </button>
          <span className="font-mono text-sm text-ink" aria-live="polite">{size}%</span>
          <button onClick={() => onChange({ size: size + SIZE_STEP })} disabled={size >= SIZE_MAX} aria-label="Aumentar a letra" className={stepButton}>
            <span aria-hidden="true" className="text-xl leading-none">A</span>
          </button>
        </div>
      </Group>

      <Group label="Cor da página">
        <div className="flex items-center justify-between gap-1">
          {READING_THEMES.map((t) => (
            <button
              key={t.id}
              role="radio"
              aria-checked={t.id === theme}
              aria-label={t.label}
              title={t.label}
              onClick={() => onChange({ theme: t.id })}
              style={{ backgroundColor: t.background, color: t.text }}
              className={`flex h-11 w-11 items-center justify-center rounded-full border text-sm transition-[transform,box-shadow] duration-150 active:scale-95 ${
                t.id === theme ? 'border-brand ring-2 ring-brand/40' : 'border-border-hairline'
              }`}
            >
              <span aria-hidden="true">{t.id === theme ? '✓' : 'Aa'}</span>
            </button>
          ))}
        </div>
      </Group>

      <Group label="Fonte">
        <div className="grid grid-cols-2 gap-2">
          {READING_FONTS.map((f) => (
            <button key={f.id} role="radio" aria-checked={f.id === font} onClick={() => onChange({ font: f.id })} style={FONT_STYLE[f.id]} className={choice(f.id === font)}>
              {f.id === 'livro' ? bookLabel : f.label}
            </button>
          ))}
        </div>
      </Group>

      <Group label="Entrelinha">
        <div className="flex gap-2">
          {READING_SPACING.map((p) => (
            <button key={p.id} role="radio" aria-checked={p.id === spacing} onClick={() => onChange({ spacing: p.id })} className={choice(p.id === spacing)}>
              {p.id === 'livro' ? bookLabel : p.label}
            </button>
          ))}
        </div>
      </Group>

      <Group label="Margens">
        <div className="grid grid-cols-2 gap-2">
          {READING_MARGINS.map((m) => (
            <button key={m.id} role="radio" aria-checked={m.id === margins} onClick={() => onChange({ margins: m.id })} className={choice(m.id === margins)}>
              {m.id === 'livro' ? bookLabel : m.label}
            </button>
          ))}
        </div>
      </Group>

      <button
        role="switch"
        aria-checked={justify}
        onClick={() => onChange({ justify: !justify })}
        className="flex min-h-11 items-center justify-between gap-3 rounded-lg border border-border-hairline bg-white px-3 text-sm text-ink transition-[background-color] duration-150 hover:bg-surface-alt"
      >
        <span>Justificar o texto</span>
        <span aria-hidden="true" className={`flex h-6 w-10 items-center rounded-full p-0.5 transition-colors duration-150 ${justify ? 'bg-brand' : 'bg-border-hairline'}`}>
          <span className={`size-5 rounded-full bg-white shadow transition-transform duration-150 ${justify ? 'translate-x-4' : ''}`} />
        </span>
      </button>
    </div>
  );
}
