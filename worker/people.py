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


def parse_name(raw):
    """(name, family, given, changed): the name people say and, only when the writing says for certain which
    words are the surname (the catalogue's way with a role: "Herbert, Frank, author"), the surname and the
    given names. Anything else is left undivided (#64)."""
    raw = raw or ''
    clean = ' '.join(raw.split())
    if not clean:
        return '', None, None, raw != ''
    name, had_role = clean, False
    match = re.match(r'^(.+?)\s*\(([^()]*)\)$', name)
    if match and _fold(match.group(2)) in ROLES:
        name, had_role = match.group(1).strip(), True
    parts = _parts(name)
    if len(parts) > 1 and _fold(parts[-1]) in ROLES:
        parts, had_role = parts[:-1], True
    if not had_role:
        return clean, None, None, clean != raw
    if len(parts) == 1:
        return parts[0], None, None, parts[0] != raw
    if len(parts) == 2:  # "Sobrenome, Nome"
        name = f'{parts[1]} {parts[0]}'
        return name, parts[0], parts[1], name != raw
    # more than that is a list, or a name with a suffix: not clear, so not touched
    return clean, None, None, clean != raw


def normalize_name(raw):
    """(the name a person goes by, whether it differs from what was written)."""
    name, _family, _given, changed = parse_name(raw)
    return name, changed


def name_key(name):
    """A name as a set of words, whichever order they come in and without role words or accents: what two
    spellings of a person have in common (the same rule as backend/internal/people.Key)."""
    words = re.split(r'[^0-9a-z]+', _fold((name or '').replace(',', ' ')))
    return ' '.join(sorted(w for w in words if w and w not in ROLES))


# What a provider calls what a person did, and the role the library keeps. A role the library has no word
# for (a colorist, a letterer) is left out: it is not made into something it is not.
PROVIDER_ROLES = {
    'author': 'author', 'writer': 'author', 'creator': 'author', 'script': 'author', 'story': 'author',
    'editor': 'editor',
    'illustrator': 'illustrator', 'penciller': 'illustrator', 'penciler': 'illustrator', 'artist': 'illustrator',
    'inker': 'illustrator', 'cover': 'illustrator',
    'translator': 'translator',
    'narrator': 'narrator',
}


def library_roles(provider_role):
    """The roles of the library that a provider's role stands for, in order and with no repeats. A provider
    that says nothing about the role is saying the person is an author of the work; one that lists several
    ("writer, inker") says each."""
    if not (provider_role or '').strip():
        return ['author']
    roles = []
    for word in re.split(r'[,/;&]| and ', provider_role.lower()):
        role = PROVIDER_ROLES.get(word.strip())
        if role and role not in roles:
            roles.append(role)
    return roles
