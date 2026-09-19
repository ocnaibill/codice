const QUOTES = [
  {
    text: 'Não terei medo. O medo é o assassino da mente. Encararei meu medo. Permitirei que passe por cima de mim e através de mim.',
    source: 'Duna, Frank Herbert',
  },
  {
    text: 'Não quero pertencer a ninguém além de mim mesma. Quero ser livre, para amar ou ficar sozinha, mas que seja por escolha própria.',
    source: 'A Vida Invisível de Addie LaRue, V.E. Schwab',
  },
];

const quote = QUOTES[Math.floor(Math.random() * QUOTES.length)];

export function AuthCard({ title, subtitle, children }) {
  return (
    <div className="flex min-h-screen w-full flex-col items-center justify-center gap-8 bg-surface px-4 py-12">
      <div className="flex flex-col items-center gap-1">
        <span className="font-display text-3xl font-semibold tracking-[-0.6px] text-brand">Códice</span>
        <span className="font-body text-[11px] tracking-[0.55px] uppercase text-ink-soft">acervo e arquivamento</span>
      </div>

      <div className="w-full max-w-md rounded-lg bg-white p-8 shadow-[0px_4px_6px_-1px_rgba(0,0,0,0.1),0px_2px_4px_-2px_rgba(0,0,0,0.1)]">
        <div className="mb-6 text-center">
          <h1 className="font-display text-xl text-ink">{title}</h1>
          {subtitle && <p className="mt-1 font-body text-[13px] text-ink-soft">{subtitle}</p>}
        </div>
        {children}
      </div>

      <blockquote className="max-w-md text-center">
        <p className="font-body text-[13px] italic leading-relaxed text-ink-soft">“{quote.text}”</p>
        <p className="mt-1 font-body text-[11px] italic text-ink-faint">— {quote.source}</p>
      </blockquote>
    </div>
  );
}
