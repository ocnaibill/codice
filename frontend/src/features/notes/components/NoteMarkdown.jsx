import React, { useMemo, useState } from 'react';
import ReactMarkdown, { defaultUrlTransform } from 'react-markdown';
import { reason } from '../../reader/noteText';
import { useCreateConcept } from '../api/useCreateConcept';
import { findLinks, linkIndex, linkify } from '../wikilinks';

/** A [[link]] to a concept the person has: a mark with what it is called and a hint of what it is. */
function ResolvedLink({ concept, children }) {
  const hint = concept.description ? `${concept.name}: ${concept.description}` : concept.name;
  return (
    <span className="rounded bg-brand/10 px-1 text-brand" title={hint} data-concept={concept.conceptId}>
      {children}
    </span>
  );
}

/** A [[link]] to a concept that does not exist yet. The text stays; nothing is created until the person says so. */
function PendingLink({ name, children }) {
  const [open, setOpen] = useState(false);
  const create = useCreateConcept();
  return (
    <span>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        title="Esse conceito ainda não existe"
        className="border-b border-dashed border-ink-soft text-ink-soft hover:text-ink"
      >
        {children}
      </button>
      {open && (
        <span className="mt-1 flex flex-wrap items-center gap-2 rounded-md border border-border-hairline bg-white px-2 py-1 text-xs">
          <span>Ainda não há um conceito “{name}”.</span>
          <button
            type="button"
            onClick={() => create.mutate(name)}
            disabled={create.isPending}
            className="rounded bg-brand px-2 py-1 font-semibold text-white hover:bg-brand-light disabled:opacity-40"
          >
            Criar conceito
          </button>
          {create.isError && <span className="text-danger">{reason(create.error, 'Não foi possível criar o conceito.')}</span>}
        </span>
      )}
    </span>
  );
}

/**
 * The text of a note, in Markdown, shown as text (raw HTML in it is not interpreted). A [[Concept]] in it is shown as
 * what it is: a mark when the person has the concept (`links` says so, by the name as written), a dashed one when
 * they do not (pending: the text is not changed and no concept is made). One whose state the server did not say is
 * shown as plain text.
 */
export function NoteMarkdown({ body, links = {} }) {
  const found = useMemo(() => findLinks(body), [body]);
  const text = useMemo(() => linkify(body, found), [body, found]);
  const components = {
    a: ({ node: _node, href, children, ...rest }) => {
      const n = linkIndex(href);
      if (n < 0 || !found[n]) {
        return (
          <a href={href} {...rest}>
            {children}
          </a>
        );
      }
      const concept = links[found[n].name];
      if (concept) return <ResolvedLink concept={concept}>{children}</ResolvedLink>;
      if (concept === null) return <PendingLink name={found[n].name}>{children}</PendingLink>;
      return <span>{children}</span>;
    },
  };
  return (
    <ReactMarkdown
      components={components}
      urlTransform={(url) => (linkIndex(url) >= 0 ? url : defaultUrlTransform(url))}
    >
      {text}
    </ReactMarkdown>
  );
}
