import React from 'react';
import { LibraryIcon } from '../../../components/ui/LibraryIcon';

const SIZES = [14, 16, 18, 20, 22, 26];
const DEFAULT_SIZE = 2; // 18 px
const STORAGE_KEY = 'codice:work-preview';

/** How the person last set the preview: kept on the device, and the preview is the same without it. */
function readStyle() {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
    return {
      size: Number.isInteger(saved.size) && saved.size >= 0 && saved.size < SIZES.length ? saved.size : DEFAULT_SIZE,
      mono: saved.mono === true,
      dark: saved.dark === true,
    };
  } catch {
    return { size: DEFAULT_SIZE, mono: false, dark: false };
  }
}

function saveStyle(style) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(style));
  } catch {
    // not kept: it is the same for this visit
  }
}

/** The paragraphs of a passage: the index keeps them with a blank line between. */
function paragraphsOf(text) {
  return String(text || '').split(/\n{2,}/).map((p) => p.replace(/\s*\n\s*/g, ' ').trim()).filter(Boolean);
}

const TOOL = 'flex min-h-9 min-w-9 items-center justify-center rounded-md border border-border-hairline bg-white px-2 text-xs text-ink hover:bg-surface-alt disabled:opacity-40';

/**
 * A plain preview of the text of the work, from the index (DEC-153): the passages around where the person is, with the type the person
 * likes (size, serif or monospaced, light or dark) and a way to go back and forth by windows and to open the reader there. It is not the
 * reader: it has no layout of the book, no marks, no position of its own to save.
 */
export function WorkPreview({ title, data, onPrev, onNext, onOpen, busy = false }) {
  const [style, setStyle] = React.useState(readStyle);
  const [full, setFull] = React.useState(false);
  const frame = React.useRef(null);

  React.useEffect(() => {
    const change = () => setFull(document.fullscreenElement === frame.current);
    document.addEventListener('fullscreenchange', change);
    return () => document.removeEventListener('fullscreenchange', change);
  }, []);

  const set = (next) => {
    const merged = { ...style, ...next };
    setStyle(merged);
    saveStyle(merged);
  };
  const toggleFull = () => {
    if (document.fullscreenElement) document.exitFullscreen?.();
    else frame.current?.requestFullscreen?.();
  };

  const segments = data?.segments ?? [];
  if (!data || data.total === 0 || segments.length === 0) return null;

  const first = segments[0];
  const lastSeq = segments[segments.length - 1].sequence;
  const here = data.current != null && data.current >= first.sequence && data.current <= lastSeq;
  const target = here ? segments.find((s) => s.sequence === data.current) ?? first : first;
  const position = `Trecho ${first.sequence + 1}${lastSeq > first.sequence ? `–${lastSeq + 1}` : ''} de ${data.total}`;

  // A chapter's name is said where it starts, not above every passage.
  let shown = null;
  const blocks = segments.map((segment) => {
    const heading = segment.chapter && segment.chapter !== shown ? segment.chapter : null;
    if (segment.chapter) shown = segment.chapter;
    return { segment, heading };
  });

  return (
    <section ref={frame} aria-label="Pré-visualização do texto" className={`flex flex-col gap-3 rounded-xl border border-border-hairline p-4 shadow-sm ${style.dark ? 'bg-ink text-surface' : 'bg-white text-ink'} ${full ? 'overflow-y-auto' : ''}`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="font-display text-2xl">Pré-visualização do texto</h2>
          <p className={`text-xs ${style.dark ? 'text-surface/70' : 'text-ink-faint'}`}>Um trecho simples, do índice de texto: o leitor tem o livro como ele é.</p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Aparência do trecho">
          <button type="button" className={TOOL} aria-label="Diminuir a letra" disabled={style.size <= 0} onClick={() => set({ size: style.size - 1 })}>A−</button>
          <button type="button" className={TOOL} aria-label="Aumentar a letra" disabled={style.size >= SIZES.length - 1} onClick={() => set({ size: style.size + 1 })}>A+</button>
          <button type="button" className={TOOL} aria-pressed={!style.mono} onClick={() => set({ mono: false })}>Serifada</button>
          <button type="button" className={TOOL} aria-pressed={style.mono} onClick={() => set({ mono: true })}>Monoespaçada</button>
          <button type="button" className={TOOL} aria-pressed={style.dark} aria-label={style.dark ? 'Fundo claro' : 'Fundo escuro'} title={style.dark ? 'Fundo claro' : 'Fundo escuro'} onClick={() => set({ dark: !style.dark })}>
            <LibraryIcon name="clock" />
          </button>
          <button type="button" className={TOOL} aria-pressed={full} onClick={toggleFull}>{full ? 'Sair da tela cheia' : 'Tela cheia'}</button>
        </div>
      </div>

      <article
        className={`mx-auto w-full max-w-prose rounded-lg px-2 py-3 leading-relaxed ${style.mono ? 'font-mono' : 'font-display'} ${busy ? 'opacity-60' : ''}`}
        style={{ fontSize: `${SIZES[style.size]}px` }}
        aria-busy={busy}
      >
        <p className={`mb-3 font-mono text-[10px] uppercase tracking-widest ${style.dark ? 'text-surface/60' : 'text-ink-faint'}`}>{title}</p>
        {blocks.map(({ segment, heading }) => (
          <div key={segment.sequence} data-here={segment.sequence === data.current ? 'true' : undefined}>
            {heading && <h3 className="mb-2 mt-4 font-display text-[1.15em] font-semibold">{heading}</h3>}
            {paragraphsOf(segment.text).map((paragraph, i) => (
              <p key={i} className={`mb-3 ${segment.sequence === data.current ? 'border-l-2 border-brand pl-3' : ''}`}>{paragraph}</p>
            ))}
          </div>
        ))}
      </article>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-mono text-[11px]" aria-live="polite">
          {position}
          {here ? ' · onde você está' : ''}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className={TOOL} disabled={data.prevFrom == null || busy} onClick={() => onPrev(data.prevFrom)}>Anterior</button>
          <button type="button" className={TOOL} disabled={data.nextFrom == null || busy} onClick={() => onNext(data.nextFrom)}>Próxima</button>
          <button type="button" className="min-h-9 rounded-md bg-brand px-3 text-xs font-semibold text-white hover:bg-brand-light" onClick={() => onOpen(target)}>
            Abrir no leitor
          </button>
        </div>
      </div>
    </section>
  );
}
