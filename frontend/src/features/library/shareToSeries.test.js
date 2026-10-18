import { describe, expect, it } from 'vitest';
import { SHARE_FIELDS, previewLines, shareRequest } from './shareToSeries';

describe('shareRequest', () => {
  it('turns the choices into the body the server reads', () => {
    expect(shareRequest({ source: 7, fields: ['publisher', 'tags'], where: 'volume', mode: 'fill', preview: true })).toEqual({
      sourceWorkId: 7, fields: ['publisher', 'tags'], units: ['volume'], mode: 'fill', preview: true,
    });
    expect(shareRequest({ source: 7, fields: ['authors'], where: 'all', mode: 'replace', preview: false }).units).toEqual([]);
  });

  it('offers the fields the server shares and not the ones of each file', () => {
    const names = SHARE_FIELDS.map(([id]) => id);
    expect(names).toContain('publisher');
    for (const own of ['title', 'isbn', 'publication_date', 'cover', 'series_index']) expect(names).not.toContain(own);
  });
});

describe('previewLines', () => {
  it('says how many change, what is kept and what the work has none of', () => {
    const lines = previewLines({ works: 38, worksChanged: 26, kept: { publisher: 12 }, sourceHasNone: ['description'] }, 'fill');
    expect(lines[0]).toBe('26 obras mudam, de 38 outras da série.');
    expect(lines[1]).toBe('Ficam como estão, por já terem outro valor: Editora (12).');
    expect(lines[2]).toBe('Esta obra não tem Sinopse: não há o que levar.');
  });

  it('says it in the singular, and says when nothing changes', () => {
    expect(previewLines({ works: 1, worksChanged: 1 }, 'replace')).toEqual(['1 obra muda, de 1 outra da série.']);
    expect(previewLines({ works: 3, worksChanged: 0, kept: { publisher: 3 } }, 'replace')).toEqual(['Nenhuma das 3 outras obras da série muda.']);
  });
});
