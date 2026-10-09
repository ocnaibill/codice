import { describe, it, expect } from 'vitest';
import { MAX_DEPTH, inOrder, descendantsOf, heightOf, placesFor, pathOf, ancestorsOf } from './tree';

// As the server sends it: flat, sorted by name.
const flat = [
  { id: 1, parentId: null, name: 'Ficção científica' },
  { id: 2, parentId: null, name: 'Mangá' },
  { id: 3, parentId: 2, name: 'Seinen' },
  { id: 4, parentId: 2, name: 'Shounen' },
  { id: 5, parentId: 3, name: 'Dark' },
];

describe('inOrder', () => {
  it('puts each category before what is under it, with how deep it is', () => {
    expect(inOrder(flat).map((c) => [c.name, c.depth])).toEqual([
      ['Ficção científica', 1], ['Mangá', 1], ['Seinen', 2], ['Dark', 3], ['Shounen', 2],
    ]);
  });
  it('says which have something under them', () => {
    const by = Object.fromEntries(inOrder(flat).map((c) => [c.name, c.hasChildren]));
    expect(by).toEqual({ 'Ficção científica': false, Mangá: true, Seinen: true, Dark: false, Shounen: false });
  });
  it('puts at the top one whose parent is not in the list', () => {
    expect(inOrder([{ id: 9, parentId: 77, name: 'Órfã' }]).map((c) => [c.name, c.depth])).toEqual([['Órfã', 1]]);
  });
  it('keeps what it was given, and an empty tree is empty', () => {
    expect(inOrder([])).toEqual([]);
    expect(inOrder(flat)[1]).toMatchObject({ id: 2, name: 'Mangá' });
  });
});

describe('descendantsOf', () => {
  it('is everything under a category, at any depth, and not the category', () => {
    expect([...descendantsOf(flat, 2)].sort()).toEqual([3, 4, 5]);
    expect([...descendantsOf(flat, 3)]).toEqual([5]);
  });
  it('is empty for a category with nothing under it, and for one that is not there', () => {
    expect(descendantsOf(flat, 1).size).toBe(0);
    expect(descendantsOf(flat, 99).size).toBe(0);
  });
});

describe('heightOf', () => {
  it('counts the levels of a category and what is under it', () => {
    expect(heightOf(flat, 1)).toBe(1);
    expect(heightOf(flat, 2)).toBe(3);
    expect(heightOf(flat, 3)).toBe(2);
    expect(heightOf(flat, 5)).toBe(1);
  });
});

describe('placesFor', () => {
  const ids = (list) => list.map((c) => c.id);
  it('leaves out the category and what is under it', () => {
    // Mangá has three levels with what is under it, so it fits nowhere but at the top (that is always possible, and is not in the list).
    expect(ids(placesFor(flat, 2))).toEqual([]);
    expect(ids(placesFor(flat, 1))).toEqual([2, 3, 4]);
  });
  it('leaves out the places that would take the tree past the limit', () => {
    // Dark is one level: it can go under any category of the first two levels, not under another of the third (there is none else).
    expect(ids(placesFor(flat, 5))).toEqual([1, 2, 3, 4]);
    // Seinen (two levels with Dark) cannot go under a category of the second level, which would make four. Its own parent is a place.
    expect(ids(placesFor(flat, 3))).toEqual([1, 2]);
  });
  it('offers, for a new category, the first two levels', () => {
    expect(ids(placesFor(flat, null))).toEqual([1, 2, 3, 4]);
    expect(MAX_DEPTH).toBe(3);
  });
  it('is empty for an empty tree', () => {
    expect(placesFor([], null)).toEqual([]);
  });
});

describe('pathOf', () => {
  it('says where a category is, from the top', () => {
    expect(pathOf(flat, 1)).toBe('Ficção científica');
    expect(pathOf(flat, 3)).toBe('Mangá › Seinen');
    expect(pathOf(flat, 5)).toBe('Mangá › Seinen › Dark');
  });
  it('is empty for one that is not there', () => {
    expect(pathOf(flat, 99)).toBe('');
  });
});

describe('ancestorsOf', () => {
  it('is the categories above one, from the top down, and not the category itself', () => {
    expect(ancestorsOf(flat, 5).map((c) => c.name)).toEqual(['Mangá', 'Seinen']);
    expect(ancestorsOf(flat, 3).map((c) => c.name)).toEqual(['Mangá']);
  });
  it('is empty for one at the top and for one that is not there', () => {
    expect(ancestorsOf(flat, 1)).toEqual([]);
    expect(ancestorsOf(flat, 99)).toEqual([]);
  });
});
