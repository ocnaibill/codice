import React from 'react';
import { useRating } from '../../reader/api/useRating';

const STARS = [1, 2, 3, 4, 5];

/**
 * The stars the person gives the work (DEC-154): whole stars, from 1 to 5, that only they see. Pressing the star they gave takes the rating
 * away; pressing another changes it.
 */
export function WorkRating({ workId, rating = 0 }) {
  const rate = useRating(workId);
  return (
    <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Sua nota">
      <span className="font-mono text-[11px] uppercase tracking-widest text-ink-faint">Sua nota</span>
      <span className="flex items-center">
        {STARS.map((n) => {
          const on = n <= rating;
          const label = n === rating ? `Tirar sua nota de ${n} ${n === 1 ? 'estrela' : 'estrelas'}` : `Dar ${n} ${n === 1 ? 'estrela' : 'estrelas'}`;
          return (
            <button
              key={n}
              type="button"
              aria-label={label}
              title={label}
              aria-pressed={n === rating}
              disabled={rate.isPending}
              onClick={() => rate.mutate(n === rating ? 0 : n)}
              className={`min-h-9 min-w-9 text-xl leading-none disabled:opacity-50 ${on ? 'text-brand' : 'text-ink-faint hover:text-brand'}`}
            >
              <span aria-hidden="true">{on ? '★' : '☆'}</span>
            </button>
          );
        })}
      </span>
      {rating > 0 && <span className="text-xs text-ink-soft">{rating} de 5</span>}
    </div>
  );
}
