// Formulas in notes (#21, DEC-111): LaTeX between $$ and $$, drawn by KaTeX in the browser. The server keeps the text as
// written and the export carries it as it is. This module is loaded only when a note has a formula, because KaTeX and its
// fonts are not small.
import 'katex/dist/katex.min.css';
import rehypeKatex from 'rehype-katex';
import remarkMath from 'remark-math';

// A single $ is a dollar sign ("R$ 5 e R$ 10" holds no formula): a formula takes two.
export const remarkPlugins = [[remarkMath, { singleDollarTextMath: false }]];

// KaTeX does not trust what it is given (trust: false): \href, \url, \includegraphics and the like are not drawn, they
// show up as red text, so a note cannot make a link or fetch a file. A formula that loops on itself or asks for a huge
// size is cut short (maxExpand, maxSize) and a wrong one is shown as the text that was written, in red, with the reason
// in its title (rehype-katex does that itself, it takes no throwOnError): neither breaks the note.
export const rehypePlugins = [
  [rehypeKatex, { strict: 'ignore', trust: false, maxExpand: 200, maxSize: 10 }],
];
