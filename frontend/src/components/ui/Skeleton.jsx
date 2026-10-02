import React from 'react';

/**
 * A shape where something is being loaded, in place of "Carregando…": the same place on the screen, a soft light
 * passing over it. Without motion (a person who asked for less) it is a still block.
 */
export function Skeleton({ className = '', label }) {
  return (
    <div
      role="status"
      aria-label={label}
      aria-busy="true"
      className={`animate-shimmer rounded-lg bg-[length:200%_100%] bg-[linear-gradient(90deg,var(--color-surface-alt)_30%,#fff_50%,var(--color-surface-alt)_70%)] ${className}`}
    />
  );
}
