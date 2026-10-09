import { describe, it, expect } from 'vitest';
import { wordsOf, placesOf, initialPlaces, partsOf, familyFirst } from './nameParts';

describe('wordsOf', () => {
  it('reads the words of a name, whatever the spaces and the commas', () => {
    expect(wordsOf('Frank Herbert')).toEqual(['Frank', 'Herbert']);
    expect(wordsOf('  Frank   Herbert ')).toEqual(['Frank', 'Herbert']);
    expect(wordsOf('Herbert, Frank')).toEqual(['Herbert', 'Frank']);
    expect(wordsOf('Herbert,Frank')).toEqual(['Herbert', 'Frank']);
    expect(wordsOf('')).toEqual([]);
    expect(wordsOf(null)).toEqual([]);
    expect(wordsOf(undefined)).toEqual([]);
  });
});

describe('placesOf', () => {
  const words = ['Gabriel', 'García', 'Márquez'];
  it('finds the run of words that spells a part', () => {
    expect(placesOf(words, 'Márquez')).toEqual([2]);
    expect(placesOf(words, 'García Márquez')).toEqual([1, 2]);
    expect(placesOf(words, 'Gabriel')).toEqual([0]);
    expect(placesOf(words, 'Gabriel García Márquez')).toEqual([0, 1, 2]);
  });
  it('finds nothing for a part that is not in the name, is out of order, is empty or is longer than the name', () => {
    expect(placesOf(words, 'Marquez')).toEqual([]);
    expect(placesOf(words, 'Márquez García')).toEqual([]);
    expect(placesOf(words, '')).toEqual([]);
    expect(placesOf(words, 'Gabriel García Márquez Jr')).toEqual([]);
    expect(placesOf([], 'x')).toEqual([]);
  });
  it('takes the first place when the word is repeated', () => {
    expect(placesOf(['Ana', 'Ana', 'Silva'], 'Ana')).toEqual([0]);
  });
});

describe('initialPlaces', () => {
  it('starts from the surname proposed, or from the one the person already has', () => {
    expect(initialPlaces({ name: 'Frank Herbert', suggestion: { family: 'Herbert', given: 'Frank' } })).toEqual([1]);
    expect(initialPlaces({ name: 'Herbert, Frank', suggestion: { family: 'Herbert', given: 'Frank' } })).toEqual([0]);
    expect(initialPlaces({ name: 'Gabriel García Márquez', family: 'García Márquez', given: 'Gabriel' })).toEqual([1, 2]);
  });
  it('starts from nothing when there is no proposal and no surname', () => {
    expect(initialPlaces({ name: 'Alan Moore & Dave Gibbons', suggestion: null })).toEqual([]);
    expect(initialPlaces({ name: 'Médicos Sem Fronteiras', undivided: true })).toEqual([]);
    expect(initialPlaces({ name: 'Frank Herbert' })).toEqual([]);
  });
  it('prefers the proposal to the surname when both are there', () => {
    expect(initialPlaces({ name: 'Frank Herbert', suggestion: { family: 'Frank' }, family: 'Herbert' })).toEqual([0]);
  });
});

describe('partsOf', () => {
  it('makes the surname and the given names of the chosen places, each in the order of the name', () => {
    expect(partsOf('Frank Herbert', [1])).toEqual({ family: 'Herbert', given: 'Frank' });
    expect(partsOf('Gabriel García Márquez', [1, 2])).toEqual({ family: 'García Márquez', given: 'Gabriel' });
    expect(partsOf('Gabriel García Márquez', [2, 1])).toEqual({ family: 'García Márquez', given: 'Gabriel' });
    expect(partsOf('Ursula K. Le Guin', [2, 3])).toEqual({ family: 'Le Guin', given: 'Ursula K.' });
    expect(partsOf('Herbert, Frank', [0])).toEqual({ family: 'Herbert', given: 'Frank' });
  });
  it('has no given names when every word is of the surname, and no surname when none is', () => {
    expect(partsOf('Médicos Sem Fronteiras', [0, 1, 2])).toEqual({ family: 'Médicos Sem Fronteiras', given: '' });
    expect(partsOf('Frank Herbert', [])).toEqual({ family: '', given: 'Frank Herbert' });
  });
});

describe('familyFirst', () => {
  it('writes the surname first, with a comma when there are given names', () => {
    expect(familyFirst({ family: 'Herbert', given: 'Frank' })).toBe('Herbert, Frank');
    expect(familyFirst({ family: 'Médicos Sem Fronteiras', given: '' })).toBe('Médicos Sem Fronteiras');
  });
});
