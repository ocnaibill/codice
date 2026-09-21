"""The shape of a book, the same for every format: front matter, body, back matter (DEC-087).

A reader of the text index (the equivalent-position engine) needs to know, of any file, where the
chapters are and what is not part of the story: a preface, a table of contents, an appendix that
names every character. The formats say it in their own ways (an EPUB's table of contents, a PDF's
outline), so each reader turns what it finds into a list of nodes and this module gives every node
its part. A node is a dict:

    {'title': str, 'depth': int, 'chars': int, 'part': 'front' | 'body' | 'back'}

`depth` is 0 for the top of the outline. `chars` is how much text the node holds by itself, which
tells a title page from a chapter.

Nothing here reads a language: a few words in Portuguese, English, Spanish and French say what a
node obviously is, and where they say nothing the place of the node decides. A file with no outline
has no nodes, and that is left as it is: no structure is not made up.
"""
import re
import unicodedata

FRONT, BODY, BACK = 'front', 'body', 'back'

# Text a node must hold, with what is under it, to be part of the story rather than a title page, and
# how much of what its neighbours typically hold: in a book divided in three parts of 400,000
# characters, a closing note of 1,500 is not a part of the story.
SUBSTANTIAL_CHARS = 1500
SUBSTANTIAL_SHARE = 0.1


def _fold(text):
    text = unicodedata.normalize('NFD', text or '')
    # Without the accents (and the vowel marks of Arabic and the tatweel that stretches a word), an alef
    # with a hamza is an alef: the writing of a title does not tell two spellings apart.
    return ''.join(c for c in text if unicodedata.category(c) != 'Mn' and c != '\u0640').lower()


def _words(*words):
    """A title starts with one of these (after an article): a chapter that merely mentions "the end"
    or "notes" is a chapter."""
    return re.compile(r'^(?:the |o |a |os |as |el |la |le |les |l )?(?:' + '|'.join(words) + r')')


# What comes before the story, whatever the language.
_FRONT = _words(
    r'cover', r'capa\b', r'title\s*page', r'titulo\b', r'folha de rosto', r'copyright', r'dedicat', r'epigraph',
    r'epigrafe', r'contents', r'table of contents', r'sumario', r'indice\b', r'pref[aá]cio', r'preface', r'foreword',
    r'prologo', r'prologue', r'introduc', r'e-?text prepared', r'produced by', r'list of illustrations', r'lista de ilustra',
    r'note to the reader', r'nota (do|da|ao)', r'half.?title', r'frontispiece', r'praise for', r'also by', r'books by',
    # Arabic: title page, copyright, dedication, contents, preface, introduction
    r'(?:ال)?صفح[هة] (?:ال)?عنوان', r'(?:ال)?صفح[هة] حقوق', r'حقوق (?:ال)?طبع', r'(?:ال)?اهداء', r'(?:ال)?اخلاص', r'(?:ال)?محتويات', r'فهرس (?:ال)?محتويات', r'(?:ال)?مقدم[هة]', r'(?:ال)?تمهيد', r'(?:ال)?تقديم', r'(?:ال)?غلاف',
    r'other books', r'about this book', r'sobre este livro',
)
# What comes after it.
_BACK = _words(
    r'appendix', r'appendices', r'apendice', r'glossary', r'glossario', r'terminolog', r'bibliograph', r'index\s*$',
    r'indice remissivo', r'notes?\s*$', r'notas?\s*$', r'endnotes', r'afterword', r'posfacio', r'colophon', r'about the author',
    r'sobre o autor', r'licen[cs]e', r'errata', r'lista de erros', r'erros corrigidos', r'copyright notice',
    r'transcriber', r'nota do transcritor', r'full project gutenberg', r'project gutenberg', r'end\s*$', r'fim\s*$', r'maps?\s*$', r'mapas?\s*$', r'credits\s*$', r'creditos\s*$',
    r'permissions', r'discussion questions', r'reading group', r'excerpt from', r'sneak peek', r'newsletter',
    # Arabic: appendices, terminology, glossary, index, notes, afterword, about the author, references
    r'(?:ال)?ملاحق', r'(?:ال)?ملحق', r'(?:ال)?مصطلحات', r'(?:ال)?مسرد', r'(?:ال)?معجم', r'(?:ال)?فهرس\s*$', r'ملاحظات (?:رسم|ختامي[هة])', r'(?:ال)?خاتم[هة] بقلم', r'(?:ال)?كلم[هة] ختامي[هة]', r'عن (?:ال)?مؤلف', r'(?:ال)?مراجع\s*$', r'(?:ال)?هوامش\s*$',
)
# What may be at either end (the place decides).
_EITHER = _words(r'acknowledg', r'agradecimentos', r'remerciements', r'agradecimientos', r'شكر')


def _kind(title):
    folded = re.sub(r'^[\W_]+', '', _fold(title))  # Unicode-aware: a title in Arabic is not "nothing"
    if _EITHER.search(folded):
        return 'either'
    if _BACK.search(folded):
        return 'back'
    if _FRONT.search(folded):
        return 'front'
    return 'neutral'


def _subtree_chars(nodes):
    """For each node, the text it holds with everything nested under it."""
    totals = [n.get('chars', 0) for n in nodes]
    for i, node in enumerate(nodes):
        for later in nodes[i + 1:]:
            if later['depth'] <= node['depth']:
                break
            totals[i] += later.get('chars', 0)
    return totals


def classify(nodes):
    """Sets the 'part' of every node, in place, and returns the list."""
    if not nodes:
        return nodes
    kinds = [_kind(n['title']) for n in nodes]
    totals = _subtree_chars(nodes)
    sizes = sorted(t for k, t in zip(kinds, totals) if k == 'neutral' and t > 0)
    typical = sizes[len(sizes) // 2] if sizes else 0
    threshold = max(SUBSTANTIAL_CHARS, SUBSTANTIAL_SHARE * typical)
    substantial = [k == 'neutral' and t >= threshold for k, t in zip(kinds, totals)]

    if any(substantial):
        first = substantial.index(True)
        last = len(substantial) - 1 - substantial[::-1].index(True)
        # The last story node ends where its own children do: what follows is back matter.
        end = last + 1
        while end < len(nodes) and nodes[end]['depth'] > nodes[last]['depth']:
            end += 1
    else:
        first, end = len(nodes), len(nodes)  # nothing that reads as a story: everything is around it

    for i, node in enumerate(nodes):
        kind = kinds[i]
        if i < first:
            node['part'] = FRONT if kind != 'back' else BACK
        elif i >= end:
            node['part'] = BACK if kind != 'front' else FRONT
        elif kind == 'back':
            node['part'] = BACK
        else:
            node['part'] = BODY  # inside the story, even a chapter called "Introduction"
    # What is nested under a node that is not the story is not the story either.
    for i, node in enumerate(nodes):
        if node['part'] != BODY:
            continue
        parent = i - 1
        while parent >= 0 and nodes[parent]['depth'] >= node['depth']:
            parent -= 1
        if parent >= 0 and nodes[parent]['part'] != BODY and nodes[parent]['depth'] < node['depth']:
            node['part'] = nodes[parent]['part']
    return nodes
