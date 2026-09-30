"""What a person's name is when a file writes it in a catalogue's way (#36): "Herbert, Frank, author" is
Frank Herbert. The same rule as backend/internal/people (and the migration that fixed the stored names):
only a role word makes the catalogue's way clear, and anything else is left as it was written."""
import re
import unicodedata

# Words a catalogue puts after a name to say what the person did, in lower case and with no accents.
ROLES = {
    'author', 'autor', 'autora', 'auteur', 'autore',
    'editor', 'editora', 'editeur', 'editore',
    'illustrator', 'ilustrador', 'ilustradora', 'illustrateur', 'illustratore',
    'translator', 'tradutor', 'tradutora', 'traductor', 'traductora', 'traducteur', 'traduttore',
    'narrator', 'narrador', 'narradora', 'narrateur',
    'contributor', 'colaborador',
}


def _fold(text):
    decomposed = unicodedata.normalize('NFD', text)
    return ''.join(c for c in decomposed if unicodedata.category(c) != 'Mn').lower().strip(' .;:()[]')


def _parts(name):
    return [' '.join(p.split()) for p in name.split(',') if p.strip()]


def normalize_name(raw):
    """(the name a person goes by, whether it differs from what was written)."""
    raw = raw or ''
    clean = ' '.join(raw.split())
    if not clean:
        return '', raw != ''
    name, had_role = clean, False
    match = re.match(r'^(.+?)\s*\(([^()]*)\)$', name)
    if match and _fold(match.group(2)) in ROLES:
        name, had_role = match.group(1).strip(), True
    parts = _parts(name)
    if len(parts) > 1 and _fold(parts[-1]) in ROLES:
        parts, had_role = parts[:-1], True
    if not had_role:
        return clean, clean != raw
    if len(parts) == 1:
        name = parts[0]
    elif len(parts) == 2:  # "Sobrenome, Nome"
        name = f'{parts[1]} {parts[0]}'
    else:  # more than that is a list, or a name with a suffix: not clear, so not touched
        return clean, clean != raw
    return name, name != raw
