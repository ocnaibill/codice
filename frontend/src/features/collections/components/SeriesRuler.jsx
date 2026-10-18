import React from 'react';
import { readThrough, rulerChips, rulerLabel, spanText } from '../ruler';

const TONES = {
  ok: 'bg-success-soft text-success',
  warn: 'bg-warning-soft text-warning',
  plain: 'bg-surface-alt text-ink-soft',
};
const FILL = { read: '#2d6a3f', have: '#cfe1d5', gap: '#f3e3b3' };
const W = 1000;

/**
 * The ruler of the series (DEC-202): the chapters of its story in a line. The files that hold many chapters (volumes, editions) are on top, the
 * loose chapters under them, and the chapters themselves in the colour of what they are: read, in the library, or in no file. Tapping a work
 * opens it; the hatched stretches are what no file contains. Drawn only when some work says what it contains.
 */
export function SeriesRuler({ ruler, onOpen }) {
  const total = ruler.cells.length;
  const cell = W / total;
  const x = (n) => (n - ruler.first) * cell;
  const through = readThrough(ruler);
  const description = rulerChips(ruler).map((c) => c.text).join('. ');
  return (
    <section aria-label="Régua da série" className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <div className="flex flex-wrap gap-1.5 text-[12px]">
        {rulerChips(ruler).map((chip) => (
          <span key={chip.text} className={`rounded-full px-2.5 py-1 ${TONES[chip.tone]}`}>{chip.text}</span>
        ))}
      </div>
      <svg role="img" aria-label={`Régua dos capítulos ${ruler.first} a ${ruler.last}. ${description}`} viewBox={`0 0 ${W} 96`} className="w-full" preserveAspectRatio="none" style={{ height: 96 }}>
        <defs>
          <pattern id="ruler-gap" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
            <rect width="8" height="8" fill={FILL.gap} />
            <line x1="0" y1="0" x2="0" y2="8" stroke="#d6a640" strokeWidth="3" />
          </pattern>
        </defs>
        {ruler.volumes.map((v) => (
          <g key={v.id} onClick={() => onOpen?.(v.id)} style={{ cursor: onOpen ? 'pointer' : 'default' }}>
            <title>{`${v.title} · ${spanText(v)}`}</title>
            <rect x={x(v.from)} y="0" width={Math.max(cell, (v.to - v.from + 1) * cell - 2)} height="28" rx="3" fill={v.done ? '#2d6a3f' : '#b8662d'} />
          </g>
        ))}
        {ruler.loose.map((c) => (
          <g key={c.id} onClick={() => onOpen?.(c.id)} style={{ cursor: onOpen ? 'pointer' : 'default' }}>
            <title>{`${c.title} · ${rulerLabel(c)}`}</title>
            <rect x={x(c.from) + 0.5} y="34" width={Math.max(cell - 1, 1)} height="22" rx="2" fill={c.done ? '#2d6a3f' : '#3b6ea8'} />
          </g>
        ))}
        {ruler.cells.map((c) => (
          <rect key={c.n} x={x(c.n)} y="64" width={Math.max(cell - 0.6, 0.5)} height="26" fill={c.state === 'gap' ? 'url(#ruler-gap)' : FILL[c.state]} />
        ))}
        {through != null && <line x1={x(through + 1)} y1="60" x2={x(through + 1)} y2="94" stroke="#1a1c1f" strokeWidth="2" />}
      </svg>
      <div className="relative h-4 text-[11px] text-ink-soft" aria-hidden="true">
        {[ruler.first, ...ruler.volumes.map((v) => v.from), ruler.last].filter((n, i, all) => all.indexOf(n) === i).map((n) => (
          <span key={n} className="absolute -translate-x-1/2" style={{ left: `${Math.min(97, Math.max(2, ((n - ruler.first + 0.5) / total) * 100))}%` }}>{n}</span>
        ))}
      </div>
      <p className="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-ink-soft">
        <span><i className="mr-1 inline-block h-2.5 w-2.5 rounded-sm" style={{ background: '#b8662d' }} />Volume ou edição</span>
        <span><i className="mr-1 inline-block h-2.5 w-2.5 rounded-sm" style={{ background: '#3b6ea8' }} />Capítulo avulso</span>
        <span><i className="mr-1 inline-block h-2.5 w-2.5 rounded-sm" style={{ background: FILL.read }} />Lido</span>
        <span><i className="mr-1 inline-block h-2.5 w-2.5 rounded-sm" style={{ background: FILL.have }} />No acervo, ainda não lido</span>
        <span><i className="mr-1 inline-block h-2.5 w-2.5 rounded-sm" style={{ background: 'repeating-linear-gradient(45deg,#fbefd0,#fbefd0 3px,#d6a640 3px,#d6a640 5px)' }} />Não há arquivo com isso</span>
      </p>
    </section>
  );
}
