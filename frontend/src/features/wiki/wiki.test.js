import { describe, it, expect } from 'vitest';
import { chapterName, filterGraph, pointLabel, filterIndex, keptBack, kindCounts, layoutGraph, neighbourhood, pointOf, pointValue } from './wiki';

const book = (position, n, side = false) => ({ position, side, title: `L${position}`, chapters: Array.from({ length: n }, (_, i) => ({ chapter: i + 1 })) });

describe('what the cursor keeps back', () => {
  const books = [book(0, 10), book(1, 8), book(2, 5, true), book(3, 6)];
  it('counts the chapters after it in its book and the main-line books after that one, not the side books', () => {
    expect(keptBack(books, { book: 0, chapter: 4 })).toEqual({ chapters: 6, books: 2 });
    expect(keptBack(books, { book: 1, chapter: 8 })).toEqual({ chapters: 0, books: 1 });
    expect(keptBack(books, { book: 3, chapter: 6 })).toEqual({ chapters: 0, books: 0 });
  });
  it('keeps every book back from someone who has not started', () => {
    expect(keptBack(books, { book: -1, chapter: 0 })).toEqual({ chapters: 0, books: 3 });
  });
});

describe('the index', () => {
  const index = [
    { slug: 'a', name: 'Ána', kind: 'person', page: false, appearances: 9, aliases: ['Aninha'] },
    { slug: 'b', name: 'Bruno', kind: 'person', page: true, appearances: 2, aliases: [] },
    { slug: 'c', name: 'Casa', kind: 'place', page: false, appearances: 5, aliases: [] },
  ];
  it('counts each kind that has someone, in the order of the filters', () => {
    expect(kindCounts(index).map((k) => [k.key, k.count])).toEqual([['person', 2], ['place', 1]]);
  });
  it('puts the pages first, then the most present, and finds by any name without accents', () => {
    expect(filterIndex(index).map((e) => e.slug)).toEqual(['b', 'a', 'c']);
    expect(filterIndex(index, { kind: 'place' }).map((e) => e.slug)).toEqual(['c']);
    expect(filterIndex(index, { text: 'ANA' }).map((e) => e.slug)).toEqual(['a']);
    expect(filterIndex(index, { text: 'aninha' }).map((e) => e.slug)).toEqual(['a']);
  });
});

describe('the cursor control', () => {
  it('names a chapter by its title when there is one', () => {
    expect(chapterName({ chapter: 3, title: '' })).toBe('Capítulo 3');
    expect(chapterName({ chapter: 3, title: 'A praça' })).toBe('3. A praça');
    expect(chapterName({ chapter: 3, title: 'Capítulo 3' })).toBe('Capítulo 3');
    expect(chapterName({ chapter: 3, title: '3' })).toBe('3');
    expect(chapterName({ chapter: 54, title: '54. Em um ano sem nome', part: '' })).toBe('54. Em um ano sem nome');
    expect(chapterName({ chapter: 10, title: 'IX', part: 'Parte Um' })).toBe('Parte Um · IX');
    expect(chapterName({ chapter: 1, title: 'Epígrafe', part: 'Abertura' })).toBe('1. Epígrafe');
  });
  it('writes a point and reads it back, and nothing else is a point', () => {
    expect(pointOf(pointValue(1, 12))).toEqual({ book: 1, chapter: 12 });
    expect(pointOf('own')).toBeNull();
  });
});

const graph = {
  nodes: [
    { slug: 'a', name: 'A', kind: 'person', appearances: 9 },
    { slug: 'b', name: 'B', kind: 'person', appearances: 5 },
    { slug: 'c', name: 'C', kind: 'place', appearances: 3 },
    { slug: 'd', name: 'D', kind: 'object', appearances: 1 },
  ],
  edges: [
    { a: 'a', b: 'b', weight: 5, labels: [] },
    { a: 'c', b: 'a', weight: 2, labels: [] },
    { a: 'b', b: 'c', weight: 1, labels: [] },
  ],
};

describe('the graph', () => {
  it('takes the strongest links of an entity, from either end of the edge', () => {
    const near = neighbourhood(graph, 'a');
    expect(near.center.slug).toBe('a');
    expect(near.links.map((l) => [l.other.slug, l.weight])).toEqual([['b', 5], ['c', 2]]);
    expect(neighbourhood(graph, 'a', 1).links).toHaveLength(1);
    expect(neighbourhood(graph, 'zz')).toBeNull();
    expect(neighbourhood(null, 'a')).toBeNull();
  });

  it('filters by kind and by weight, and counts the nodes left with no link', () => {
    expect(filterGraph(graph).nodes.map((n) => n.slug)).toEqual(['a', 'b', 'c']);
    expect(filterGraph(graph).alone).toBe(1);
    expect(filterGraph(graph, { weight: 2 }).edges).toHaveLength(2);
    const people = filterGraph(graph, { kind: 'person' });
    expect(people.edges.map((e) => `${e.a}-${e.b}`)).toEqual(['a-b']);
    expect(people.alone).toBe(0);
  });

  it('lays the nodes out inside the square, apart, and the same way every time', () => {
    const first = layoutGraph(graph.nodes, graph.edges);
    const again = layoutGraph(graph.nodes, graph.edges);
    expect([...first.entries()]).toEqual([...again.entries()]);
    for (const { x, y } of first.values()) {
      expect(x).toBeGreaterThanOrEqual(0.04);
      expect(x).toBeLessThanOrEqual(0.96);
      expect(y).toBeGreaterThanOrEqual(0.04);
      expect(y).toBeLessThanOrEqual(0.96);
    }
    const points = [...first.values()];
    for (let i = 0; i < points.length; i += 1) {
      for (let j = i + 1; j < points.length; j += 1) {
        expect(Math.hypot(points[i].x - points[j].x, points[i].y - points[j].y)).toBeGreaterThan(0.05);
      }
    }
    expect(layoutGraph([], []).size).toBe(0);
  });
});

describe('a point inside the page', () => {
  it('drops the title of the book when the wiki has only one', () => {
    const one = [{ title: 'Addie' }];
    expect(pointLabel('Addie, Parte Três · IX', one)).toBe('Parte Três · IX');
    expect(pointLabel('Addie · 3', one)).toBe('3');
    expect(pointLabel('Addie, Parte Três · IX', [{ title: 'Addie' }, { title: 'Outro' }])).toBe('Addie, Parte Três · IX');
  });
});
