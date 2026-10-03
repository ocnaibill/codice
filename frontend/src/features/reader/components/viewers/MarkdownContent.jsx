import React from 'react';
import ReactMarkdown from 'react-markdown';

// What a Markdown file looks like. The colors and the size come from the page (everything is in `em` and in the color
// of the text), so that every page color and every size reads well. Raw HTML in the file is not interpreted.
const components = {
  h1: ({ node: _node, children }) => <h1 className="mb-4 mt-8 text-[2em] font-semibold leading-tight first:mt-0">{children}</h1>,
  h2: ({ node: _node, children }) => <h2 className="mb-3 mt-8 text-[1.55em] font-semibold leading-tight">{children}</h2>,
  h3: ({ node: _node, children }) => <h3 className="mb-2 mt-6 text-[1.25em] font-semibold leading-tight">{children}</h3>,
  h4: ({ node: _node, children }) => <h4 className="mb-2 mt-5 font-semibold">{children}</h4>,
  h5: ({ node: _node, children }) => <h5 className="mb-2 mt-4 font-semibold">{children}</h5>,
  h6: ({ node: _node, children }) => <h6 className="mb-2 mt-4 font-semibold opacity-80">{children}</h6>,
  p: ({ node: _node, children }) => <p className="my-[0.9em]">{children}</p>,
  a: ({ node: _node, children, href }) => <a href={href} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2" style={{ color: 'var(--reading-link)' }}>{children}</a>,
  ul: ({ node: _node, children }) => <ul className="my-[0.9em] list-disc pl-[1.5em]">{children}</ul>,
  ol: ({ node: _node, children }) => <ol className="my-[0.9em] list-decimal pl-[1.5em]">{children}</ol>,
  li: ({ node: _node, children }) => <li className="my-[0.25em]">{children}</li>,
  blockquote: ({ node: _node, children }) => <blockquote className="my-[1em] border-l-4 border-current/30 pl-[1em] opacity-90">{children}</blockquote>,
  hr: () => <hr className="my-8 border-current/20" />,
  pre: ({ node: _node, children }) => <pre className="my-[1em] overflow-x-auto rounded-lg bg-current/10 p-[1em] text-[0.9em] leading-snug [&_code]:bg-transparent [&_code]:p-0">{children}</pre>,
  code: ({ node: _node, className, children }) => <code className={`font-mono text-[0.92em] ${className ?? ''} ${className ? '' : 'rounded bg-current/10 px-[0.3em] py-[0.1em]'}`}>{children}</code>,
  img: ({ node: _node, src, alt }) => <img src={src} alt={alt ?? ''} className="my-[1em] h-auto max-w-full rounded" />,
};

export default function MarkdownContent({ children }) {
  return <ReactMarkdown components={components}>{children}</ReactMarkdown>;
}
