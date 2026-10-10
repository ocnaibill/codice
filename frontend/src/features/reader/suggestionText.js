import { languageName } from './files';

// How a series stands in publication (DEC-170), as a reader reads it.
export const PUBLICATION_LABELS = { ongoing: 'Em andamento', finished: 'Concluída', hiatus: 'Em hiato', cancelled: 'Cancelada' };

const ROLE_LABELS = { author: 'autor', illustrator: 'ilustrador', translator: 'tradutor', editor: 'editor', narrator: 'narrador' };
const roleLabel = (role) => ROLE_LABELS[role] || role;

// The server says who the work has with the library's role words; a reader reads them in Portuguese.
export const localizeRoles = (text) => text.replace(/\((author|illustrator|translator|editor|narrator)\)/g, (_, role) => `(${roleLabel(role)})`);

/** A suggestion's value as a reader reads it. A list (tags, people) comes as JSON; one that is not what its
 *  field promises is shown as it came instead of breaking the others. */
export function suggestionText(candidate) {
  const { field, value } = candidate;
  try {
    const parsed = JSON.parse(value);
    if (field === 'tags' && Array.isArray(parsed)) return parsed.join(', ');
    if (field === 'contributors' && Array.isArray(parsed)) return parsed.map((p) => `${p.name} (${roleLabel(p.role)})`).join('; ');
  } catch {
    // not JSON: shown as it is
  }
  if (field === 'language') return languageName(value) || value;
  if (field === 'series_status') return PUBLICATION_LABELS[value] || value;
  return value;
}

