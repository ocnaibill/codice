import { useRef } from 'react';
import { ago } from '../../../lib/ago';
import { useDialog } from '../../../lib/useDialog';
import { usePlaceComparison } from '../api/usePlaceComparison';

const percentText = (value) => `${Math.round(Number(value) || 0)}%`;

/** The name of a device for a button: as it is when it is short, else its first word ("Kindle de Binaco" is "Kindle"). */
const shortName = (name) => (name.length <= 12 ? name : name.split(/\s+/)[0]);

/** How long ago, for a card: "Há 18 minutos"; "Agora há pouco" for the last few minutes. */
function whenText(iso) {
  const text = ago(iso);
  if (!text) return 'Agora há pouco';
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function Icon({ d, size = 14, className = '' }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
      <path d={d} />
    </svg>
  );
}

const PATHS = {
  sync: 'M3 12a9 9 0 0 1 15-6.7L21 8M21 3v5h-5M21 12a9 9 0 0 1-15 6.7L3 16M3 21v-5h5',
  close: 'M6 6l12 12M18 6L6 18',
  clock: 'M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0',
  quote: 'M7 17h3l2-4V7H6v6h3M15 17h3l2-4V7h-6v6h3',
  book: 'M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2V5zM4 19a2 2 0 0 1 2-2h13',
  device: 'M7 3h10a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1zM11 18h2',
  laptop: 'M4 6h16v10H4zM2 20h20',
  note: 'M5 4h14v16H5zM9 9h6M9 13h6',
};

/** One of the two places: who, when, how far, where in words and the passage. `ahead` is the one that is further on. */
function PlaceCard({ place, name, icon, ahead, passageLabel, children, action }) {
  return (
    <div className="relative flex flex-col overflow-hidden rounded-lg bg-white p-5 shadow-sm">
      <div aria-hidden="true" className={`absolute right-0 top-0 h-full w-2 ${ahead ? 'bg-brand' : 'bg-surface-alt'}`} />
      <div className="flex-1">
        <div className="mb-3 flex items-center justify-between gap-2 pr-3">
          <span className={`inline-flex min-w-0 items-center gap-1.5 rounded-full px-2.5 py-1 font-mono text-[11px] font-semibold ${ahead ? 'bg-success-soft/50 text-success' : 'bg-surface-alt text-ink'}`}>
            <Icon d={PATHS[icon]} size={13} className="shrink-0" />
            <span className="truncate">{name}</span>
          </span>
          <span className="flex shrink-0 items-center gap-1 font-mono text-[11px] text-ink-soft">
            <Icon d={PATHS.clock} size={12} />
            <span title={place.at ? new Date(place.at).toLocaleString('pt-BR') : undefined}>{place.at ? whenText(place.at) : ''}</span>
          </span>
        </div>
        <div className="mb-4 space-y-1 pr-3">
          <div className="flex items-baseline gap-2">
            <span className={`font-display text-[24px] font-semibold leading-none ${ahead ? 'text-brand' : 'text-ink'}`}>{percentText(place.percent)}</span>
            <span className="font-mono text-[11px] font-medium uppercase text-ink-soft">do livro</span>
          </div>
          {place.chapter && <p className="text-[15px] font-semibold text-ink">{place.chapter}</p>}
          {place.page > 0 && <p className="font-mono text-[11px] text-ink-soft">Página {place.page}</p>}
        </div>
        {place.snippet && (
          <div className="mb-4 rounded-lg bg-surface p-3.5 pr-4">
            <div className="mb-1 flex items-center gap-1.5 font-mono text-[11px] font-medium text-ink-warm">
              <Icon d={PATHS.quote} size={13} />
              {passageLabel}
            </div>
            <p className="line-clamp-3 font-display text-[13px] italic leading-relaxed text-ink">“{place.snippet}”</p>
          </div>
        )}
        {children}
      </div>
      <div className="pr-3">{action}</div>
    </div>
  );
}

/**
 * When a device (a KOReader) sent a place while the web had a newer one, the server kept it apart (DEC-191) and the person chooses here,
 * with both in front of them: how far each is, when, the chapter and the passage. Going to the device's place opens the book there; staying
 * is staying where the web is. Closing is staying. What the server cannot say of a place (a chapter of a file with no text index, a page
 * of an EPUB) is not made up.
 */
export function PlaceConflictDialog({ fileId, title, pending, web, onGoTo, onStay }) {
  const dialogRef = useRef(null);
  useDialog(dialogRef, { onEscape: onStay });
  const { data } = usePlaceComparison(fileId);

  // What the screen already knows is drawn at once; what the server adds (chapter, passage, pages, notes) fills in when it arrives.
  const device = { percent: pending.percent, at: pending.at, device: pending.device, ...(data?.device ?? {}) };
  const here = { percent: web?.percent ?? 0, at: web?.updatedAt, ...(data?.web ?? {}) };
  const deviceName = String(device.device || '').trim() || 'Leitor externo';
  const deviceAhead = Number(device.percent) >= Number(here.percent);
  const gap = data?.gapPercent ?? Math.abs((Number(device.percent) || 0) - (Number(here.percent) || 0));

  return (
    <div
      className="fixed inset-0 z-[60] flex animate-fade-in items-center justify-center bg-ink/40 p-4 backdrop-blur-md sm:p-6"
      onClick={(event) => event.target === event.currentTarget && onStay()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby="place-conflict-title" className="relative flex max-h-[92vh] w-full max-w-3xl animate-pop-in flex-col overflow-hidden rounded-xl bg-surface shadow-2xl">
        <div aria-hidden="true" className="h-1.5 w-full shrink-0 bg-gradient-to-r from-brand to-brand-light" />
        <div className="overflow-y-auto">
          <div className="flex flex-col gap-3 px-6 pb-5 pt-7 sm:px-8">
            <div className="flex items-start justify-between gap-3">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-brand/10 text-brand">
                  <Icon d={PATHS.sync} size={20} />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-[11px] font-bold uppercase tracking-wider text-brand">Sincronização do Códice</span>
                    <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-brand" />
                  </div>
                  <h2 id="place-conflict-title" className="mt-0.5 font-display text-[24px] font-medium leading-tight tracking-tight text-ink">
                    Conflito de progresso detectado
                  </h2>
                </div>
              </div>
              <button onClick={onStay} aria-label="Fechar e ficar onde a web está" title="Fechar e ficar onde a web está" className="rounded-lg p-1.5 text-ink-soft hover:bg-surface-alt hover:text-ink">
                <Icon d={PATHS.close} size={18} />
              </button>
            </div>
            <p className="text-[15px] leading-relaxed text-ink-soft">
              Identificamos uma posição de leitura diferente registrada no seu leitor. Escolha qual ponto deseja manter para continuar sua leitura.
            </p>
            <div className="mt-2 flex flex-wrap items-center justify-between gap-3 rounded-lg bg-surface-alt px-4 py-2.5 text-[13px]">
              <div className="flex min-w-0 items-center gap-2.5">
                <Icon d={PATHS.book} size={16} className="shrink-0 text-ink-warm" />
                <span className="truncate font-semibold text-ink">{title || 'Este livro'}</span>
              </div>
              <span className="flex items-center gap-2 rounded-full bg-brand/10 px-2.5 py-1 font-mono text-[11px] font-semibold text-brand">
                Divergência de {percentText(gap)}
                {data?.gapPages > 0 && ` (+${data.gapPages} págs.)`}
              </span>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-5 bg-white/50 p-6 sm:p-8 md:grid-cols-2">
            <PlaceCard
              place={device}
              name={deviceName}
              icon="device"
              ahead={deviceAhead}
              passageLabel="Trecho onde parou:"
              action={
                <button
                  onClick={() => onGoTo(pending.locator)}
                  className="mt-2 flex w-full items-center justify-center gap-2 rounded-lg bg-brand px-4 py-3 text-[15px] font-semibold text-white shadow-sm transition-all hover:brightness-110 active:scale-[0.98]"
                >
                  <Icon d={PATHS.sync} size={18} />
                  {deviceAhead ? 'Avançar' : 'Voltar'} para {percentText(device.percent)} ({shortName(deviceName)})
                </button>
              }
            />
            <PlaceCard
              place={here}
              name="Leitor web"
              icon="laptop"
              ahead={!deviceAhead}
              passageLabel="Trecho onde você parou na web:"
              action={
                <button
                  onClick={onStay}
                  className="mt-2 flex w-full items-center justify-center gap-2 rounded-lg bg-surface-alt px-4 py-3 text-[15px] font-semibold text-ink transition-all hover:brightness-95 active:scale-[0.98]"
                >
                  Permanecer em {percentText(here.percent)} (web)
                </button>
              }
            >
              {data?.notes > 0 && (
                <p className="flex items-center gap-2 pb-4 font-mono text-[11px] text-ink-soft">
                  <Icon d={PATHS.note} size={13} />
                  <span><strong>{data.notes}</strong> {data.notes === 1 ? 'anotação' : 'anotações'} neste livro</span>
                </p>
              )}
            </PlaceCard>
          </div>
        </div>
      </div>
    </div>
  );
}
