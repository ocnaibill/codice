import { describe, it, expect } from 'vitest';
import { creditsLine, peopleOf, ROLES } from './credits';

const c = (name, role, position = 0) => ({ personId: name.length, name, role, position });

describe('who is credited on a work', () => {
  it('lists the roles in the order the library keeps them', () => {
    expect(ROLES.map((r) => r.key)).toEqual(['author', 'translator', 'narrator', 'editor', 'illustrator']);
  });

  it('gives the people of one role in their order, whatever order they come in', () => {
    const list = [c('Dois', 'translator', 1), c('Um', 'translator', 0), c('Outro', 'narrator')];
    expect(peopleOf(list, 'translator').map((p) => p.name)).toEqual(['Um', 'Dois']);
    expect(peopleOf(list, 'editor')).toEqual([]);
    expect(peopleOf(undefined, 'author')).toEqual([]);
  });

  it('says what the sheet adds to the authors, by role and in the order of the roles', () => {
    const list = [c('Narrador', 'narrator'), c('Frank Herbert', 'author'), c('Tradutora B', 'translator', 1), c('Tradutora A', 'translator', 0), c('Ilustrador', 'illustrator')];
    expect(creditsLine(list)).toBe('Tradução: Tradutora A, Tradutora B · Narração: Narrador · Ilustração: Ilustrador');
  });

  it('says nothing when only authors are credited, or nobody', () => {
    expect(creditsLine([c('Frank Herbert', 'author'), c('Brian Herbert', 'author', 1)])).toBe('');
    expect(creditsLine([])).toBe('');
    expect(creditsLine(undefined)).toBe('');
  });
});
