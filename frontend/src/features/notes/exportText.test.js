import { describe, it, expect } from 'vitest';
import { describeFilters, EXPORT_CAP } from './exportText';

describe('describeFilters', () => {
  it('says all of them when nothing narrows the notes', () => {
    expect(describeFilters({})).toBe('todas as suas anotações');
    expect(describeFilters({ q: '   ', kind: '', tag: '', work: null })).toBe('todas as suas anotações');
  });

  it('names each filter that is set, in a fixed order', () => {
    expect(describeFilters({ work: { id: 3, title: 'Duna' }, kind: 'highlight', tag: 'ideia', q: ' areia ' })).toBe(
      'obra “Duna”, só destaques, tag #ideia, texto “areia”'
    );
  });

  it('puts every kind in the plural', () => {
    expect(describeFilters({ kind: 'note' })).toBe('só notas');
    expect(describeFilters({ kind: 'bookmark' })).toBe('só marcadores');
  });

  it('knows the most one file holds, which is the same as the server', () => {
    expect(EXPORT_CAP).toBe(10000);
  });
});
