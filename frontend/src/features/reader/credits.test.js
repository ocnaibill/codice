import { describe, it, expect } from 'vitest';
import { peopleOf, ROLES } from './credits';

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
});
