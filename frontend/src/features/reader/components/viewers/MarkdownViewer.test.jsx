import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../../lib/api', () => ({ authenticatedUrl: (u) => u }));

import MarkdownViewer from './MarkdownViewer';
import { setPreferenceOwner } from '../../preferences';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const DOC = `# Título

Um parágrafo com **negrito**, *itálico*, \`código\` e um [link](https://example.test/x).

## Seção

- um
- dois

1. primeiro
2. segundo

> uma citação

\`\`\`js
const a = 1;
\`\`\`

---

![capa](https://example.test/capa.png)

<script>window.hacked = true</script>
<b>html cru</b>
`;
async function open(body = DOC, props = {}) {
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, status: 200, text: async () => body })));
  await act(async () => { root.render(<MarkdownViewer fileUrl="/files/a.md" onProgress={vi.fn()} {...props} />); });
  await flush();
}
const q = (sel) => container.querySelector(sel);

beforeEach(() => {
  localStorage.clear();
  setPreferenceOwner('ana');
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('the Markdown reader', () => {
  it('shows the headings, the lists, the quote, the rule and the code as what they are', async () => {
    await open();
    expect(q('h1').textContent).toBe('Título');
    expect(q('h2').textContent).toBe('Seção');
    expect(q('strong').textContent).toBe('negrito');
    expect(q('em').textContent).toBe('itálico');
    expect(q('ul').querySelectorAll('li').length).toBe(2);
    expect(q('ol').querySelectorAll('li').length).toBe(2);
    expect(q('blockquote').textContent).toContain('uma citação');
    expect(q('hr')).not.toBeNull();
    expect(q('pre code').textContent).toContain('const a = 1;');
    expect(q('img').getAttribute('alt')).toBe('capa');
    expect(q('img').getAttribute('src')).toBe('https://example.test/capa.png');
  });

  it('does not interpret raw HTML in the file', async () => {
    await open();
    expect(q('script')).toBeNull();
    expect(window.hacked).toBeUndefined();
    expect(container.querySelector('b')).toBeNull();
  });

  it('opens links in another tab, safely, in the color of the link of the page', async () => {
    await open();
    const a = q('a');
    expect(a.getAttribute('href')).toBe('https://example.test/x');
    expect(a.getAttribute('target')).toBe('_blank');
    expect(a.getAttribute('rel')).toBe('noopener noreferrer');
    expect(a.style.color).toBe('var(--reading-link)');
    expect(q('[data-reading-text]').style.getPropertyValue('--reading-link')).toBe('#1d4ed8');
  });

  it('follows the page color: the text and the link', async () => {
    localStorage.setItem('codice:epub-settings:ana', JSON.stringify({ theme: 'escuro' }));
    await open();
    expect(q('[data-reading-text]').style.color).toBe('rgb(228, 228, 231)');
    expect(q('[data-reading-text]').style.getPropertyValue('--reading-link')).toBe('#93c5fd');
  });

  it('is told in words when it cannot open, and when the file is empty', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 500 })));
    await act(async () => { root.render(<MarkdownViewer fileUrl="/files/a.md" onProgress={vi.fn()} />); });
    await flush();
    expect(q('[role="alert"]').textContent).toContain('Não foi possível abrir este arquivo.');
    expect(q('[role="alert"]').textContent).toContain('O servidor respondeu com o erro 500.');
    act(() => root.unmount());
    root = createRoot(container);
    await open('');
    expect(container.textContent).toContain('Este arquivo está vazio.');
  });

  it('has the same controls as the text reader: it hides them, and asks for the look of the text', async () => {
    const onImmersiveChange = vi.fn();
    await open(DOC, { onImmersiveChange });
    q('button[aria-label="Esconder os controles"]').click();
    expect(onImmersiveChange).toHaveBeenCalledWith(true);
    await act(async () => { q('button[aria-label="Aparência do texto"]').click(); });
    expect(q('[role="dialog"]')).not.toBeNull();
  });

  it('shows code that has a language, and code inside a line, each as code', async () => {
    await open('Uma `linha` e\n\n```\nsem linguagem\n```\n\n```py\nx = 1\n```\n');
    const codes = [...container.querySelectorAll('code')];
    expect(codes.length).toBe(3);
    expect(codes[0].textContent).toBe('linha');
    expect(codes[0].className).toContain('bg-current/10'); // a line of code has its own background
    expect(codes[1].parentElement.tagName).toBe('PRE');
    expect(codes[2].className).toContain('language-py');
  });

  it('shows every level of heading', async () => {
    await open('# a\n\n## b\n\n### c\n\n#### d\n\n##### e\n\n###### f\n');
    for (const n of [1, 2, 3, 4, 5, 6]) expect(q(`h${n}`), `h${n}`).not.toBeNull();
  });
});
