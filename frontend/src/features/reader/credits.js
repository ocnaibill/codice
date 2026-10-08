// The roles a person can have on a work, in the order they are listed, with how each is said (#185): the heading of the people with
// the role, and the name of the role for one person.
export const ROLES = [
  { key: 'author', heading: 'Autores', one: 'Autor' },
  { key: 'translator', heading: 'Tradução', one: 'Tradutor' },
  { key: 'narrator', heading: 'Narração', one: 'Narrador' },
  { key: 'editor', heading: 'Edição', one: 'Editor' },
  { key: 'illustrator', heading: 'Ilustração', one: 'Ilustrador' },
];

/** The people of one role, in their order. */
export function peopleOf(contributors, role) {
  return (contributors ?? []).filter((c) => c.role === role).sort((a, b) => a.position - b.position);
}

/**
 * What the sheet says besides the authors, which it already shows: "Tradução: A, B · Narração: C". Nothing when nobody else is
 * credited.
 */
export function creditsLine(contributors) {
  return ROLES.filter((r) => r.key !== 'author')
    .map((r) => {
      const names = peopleOf(contributors, r.key).map((c) => c.name);
      return names.length ? `${r.heading}: ${names.join(', ')}` : null;
    })
    .filter(Boolean)
    .join(' · ');
}
