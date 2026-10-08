import React from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';

/**
 * The authors of a work, each a button that opens the page of that person (#186). Without the authors one by one (what a card has
 * when the server did not say) it is the text, as it was. The names are the ones the account is shown.
 */
export function AuthorLinks({ authors, fallback = '' }) {
  const openPerson = useGlobalStore((state) => state.openPerson);
  if (!authors || authors.length === 0) return fallback;
  return authors.map((a, i) => (
    <React.Fragment key={`${a.id}-${i}`}>
      {i > 0 && ', '}
      <button
        type="button"
        onClick={() => openPerson(a.id)}
        title="Ver as obras desta pessoa"
        className="rounded-sm text-left underline decoration-dotted decoration-1 underline-offset-[3px] hover:decoration-solid focus-visible:decoration-solid"
      >
        {a.name}
      </button>
    </React.Fragment>
  ));
}
