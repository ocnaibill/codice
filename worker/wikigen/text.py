"""Small helpers for names and text that every stage shares."""
import difflib
import re
import unicodedata

CHARS_PER_TOKEN = 3.2

# Honorifics and ranks that are not part of who someone is ("Doutora Ana" and "Ana" are the same person). Generic, in the languages of the
# library (Portuguese, English, Spanish, French, Italian); nothing here comes from one book.
TITLE_WORDS = {
    'sr', 'sra', 'srta', 'senhor', 'senhora', 'senhorita', 'dom', 'dona', 'doutor', 'doutora', 'dr', 'dra', 'mestre', 'professor', 'professora',
    'capitao', 'general', 'coronel', 'major', 'sargento', 'rei', 'rainha', 'principe', 'princesa', 'imperador', 'imperatriz', 'lorde', 'lady',
    'padre', 'frei', 'irma', 'santo', 'santa', 'mr', 'mrs', 'ms', 'miss', 'sir', 'madam', 'lord', 'king', 'queen', 'prince', 'princess', 'captain',
    'father', 'brother', 'sister', 'saint', 'professor', 'master', 'senor', 'senora', 'don', 'monsieur', 'madame', 'mademoiselle', 'signor',
    'signora', 'maestro',
}
CONNECTORS = {'de', 'da', 'do', 'das', 'dos', 'e', 'a', 'o', 'os', 'as', 'na', 'no', 'em', 'para', 'por', 'com', 'del', 'la', 'le', 'von', 'van',
              'the', 'of', 'and', 'el', 'los', 'las', 'des', 'du', 'et', 'di', 'il'}


def fold(s):
    return unicodedata.normalize('NFKD', str(s).lower()).encode('ascii', 'ignore').decode()


def key(s):
    """What two renderings of a name share, as the backend keeps it for the decisions of the staff (wiki.Key): no accents, no case, no
    punctuation, single spaces. Not `fold`, which drops what is not ASCII."""
    base = ''.join(c for c in unicodedata.normalize('NFKD', str(s).lower()) if not unicodedata.combining(c))
    return re.sub(r'[^\w]+|_+', ' ', base).strip()


def norm(text):
    return re.sub(r'[\s"“”\'‘’«»—–-]+', ' ', str(text).lower()).strip()


def est_tokens(text):
    return int(len(text) / CHARS_PER_TOKEN)


def _is_title(t):
    if t in TITLE_WORDS:
        return True
    # a model misspells a title: near one, it is the title
    return len(t) >= 6 and any(difflib.SequenceMatcher(None, t, w).ratio() >= 0.86 for w in TITLE_WORDS if len(w) >= 6)


def core(name):
    """The words that identify, without titles and connectors: "Doutora Ana de Souza" → (ana, souza)."""
    toks = [t for t in re.split(r'[^a-z0-9]+', fold(name)) if t]
    return tuple(t for t in toks if t not in CONNECTORS and not _is_title(t))


KINDS = [('personagem', ['personagem', 'pessoa', 'ia', 'inteligencia', 'character', 'person']),
         ('lugar', ['lugar', 'local', 'cidade', 'regiao', 'territorio', 'place', 'location']),
         ('organização', ['organiza', 'grupo', 'instituicao', 'ordem', 'faccao', 'group']),
         ('evento', ['evento', 'conclave', 'ritual', 'event']),
         ('cargo', ['cargo', 'titulo', 'funcao', 'posicao', 'title', 'role']),
         ('objeto', ['objeto', 'artefato', 'item', 'anel', 'arma', 'object']),
         ('conceito', ['conceito', 'epoca', 'periodo', 'era', 'lei', 'regra', 'tecnologia', 'sistema', 'concept'])]


def kind_of(tipo):
    t = fold(tipo or '')
    for name, words in KINDS:
        if any(w in t for w in words):
            return name
    return 'outro'


def slugify(name, used):
    base = re.sub(r'[^a-z0-9]+', '-', fold(name)).strip('-')[:100] or 'entidade'
    slug, n = base, 2
    while slug in used:
        slug, n = f'{base}-{n}', n + 1
    used.add(slug)
    return slug
