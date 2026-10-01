import { describe, it, expect, vi } from 'vitest';
import { loadOutline, MAX_OUTLINE } from './pdfOutline';

// A PDF where page refs are objects and named destinations are strings, as PDF.js gives them.
const pdfWith = (outline, { numPages = 20, destinations = {}, pageIndex = (ref) => ref.num - 1 } = {}) => ({
  numPages,
  getOutline: vi.fn(async () => outline),
  getDestination: vi.fn(async (name) => destinations[name] ?? null),
  getPageIndex: vi.fn(async (ref) => pageIndex(ref)),
});
const at = (num) => [{ num, gen: 0 }, { name: 'XYZ' }];

describe('loadOutline', () => {
  it('flattens the outline in order, with the depth and the page of each entry from 1', async () => {
    const pdf = pdfWith([
      { title: 'Parte I', dest: at(3), items: [{ title: 'Capítulo 1', dest: at(4), items: [{ title: 'Seção', dest: at(5) }] }, { title: 'Capítulo 2', dest: at(9) }] },
      { title: 'Parte II', dest: at(15) },
    ]);
    expect(await loadOutline(pdf)).toEqual([
      { title: 'Parte I', page: 3, depth: 0 }, { title: 'Capítulo 1', page: 4, depth: 1 }, { title: 'Seção', page: 5, depth: 2 },
      { title: 'Capítulo 2', page: 9, depth: 1 }, { title: 'Parte II', page: 15, depth: 0 },
    ]);
  });

  it('resolves a named destination, and a page given as its index', async () => {
    const pdf = pdfWith(
      [{ title: 'Nomeado', dest: 'cap2' }, { title: 'Por índice', dest: [6, { name: 'Fit' }] }, { title: 'Nome ausente', dest: 'nao-existe' }],
      { destinations: { cap2: at(7) } }
    );
    expect(await loadOutline(pdf)).toEqual([{ title: 'Nomeado', page: 7, depth: 0 }, { title: 'Por índice', page: 7, depth: 0 }]);
  });

  it('leaves out what goes nowhere, and still walks the children of an entry that goes nowhere', async () => {
    const pdf = pdfWith([
      { title: 'Site', url: 'https://example.com', dest: null, items: [{ title: 'Filho', dest: at(2) }] },
      { title: '   ', dest: at(3) },
      { title: 'Sem destino' },
      { title: 'Fora do arquivo', dest: at(99) },
      { title: 'Zero', dest: [{ num: 0, gen: 0 }] },
      { title: 'Negativo', dest: [-5, {}] },
    ]);
    expect((await loadOutline(pdf)).map((e) => e.title)).toEqual(['Filho']);
  });

  it('cleans the title', async () => {
    expect(await loadOutline(pdfWith([{ title: '  Capítulo \n  Um ', dest: at(2) }]))).toEqual([{ title: 'Capítulo Um', page: 2, depth: 0 }]);
  });

  it('is empty when the PDF has no outline, or cannot say', async () => {
    expect(await loadOutline(pdfWith(null))).toEqual([]);
    expect(await loadOutline({ numPages: 3 })).toEqual([]);
    expect(await loadOutline(null)).toEqual([]);
    expect(await loadOutline({ numPages: 3, getOutline: async () => { throw new Error('broken'); } })).toEqual([]);
  });

  it('skips an entry whose destination cannot be resolved without losing the others', async () => {
    const pdf = pdfWith([{ title: 'Quebrado', dest: at(2) }, { title: 'Bom', dest: at(4) }], {
      pageIndex: (ref) => { if (ref.num === 2) throw new Error('bad ref'); return ref.num - 1; },
    });
    expect(await loadOutline(pdf)).toEqual([{ title: 'Bom', page: 4, depth: 0 }]);
  });

  it('does not go deeper than six levels, nor beyond the most it keeps', async () => {
    let deep = { title: 'N6', dest: at(2) };
    for (let i = 5; i >= 0; i--) deep = { title: `N${i}`, dest: at(2), items: [deep] };
    expect((await loadOutline(pdfWith([deep]))).map((e) => e.depth)).toEqual([0, 1, 2, 3, 4, 5, 6]);
    const tooDeep = { title: 'a', dest: at(2), items: [{ title: 'b', dest: at(2), items: [{ title: 'c', dest: at(2), items: [{ title: 'd', dest: at(2), items: [{ title: 'e', dest: at(2), items: [{ title: 'f', dest: at(2), items: [{ title: 'g', dest: at(2), items: [{ title: 'h', dest: at(2) }] }] }] }] }] }] }] };
    expect((await loadOutline(pdfWith([tooDeep]))).map((e) => e.title)).toEqual(['a', 'b', 'c', 'd', 'e', 'f', 'g']);

    const many = Array.from({ length: MAX_OUTLINE + 50 }, (_, i) => ({ title: `T${i}`, dest: at(2) }));
    expect(await loadOutline(pdfWith(many))).toHaveLength(MAX_OUTLINE);
  });

  it('accepts any page when the size of the file is not known', async () => {
    expect(await loadOutline(pdfWith([{ title: 'X', dest: at(99) }], { numPages: null }))).toEqual([{ title: 'X', page: 99, depth: 0 }]);
  });
});
