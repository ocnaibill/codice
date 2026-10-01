"""A description as plain text (#58).

The description a file carries about itself is often HTML: an EPUB's `dc:description` comes with `<b>`, `<br>` and
`&#8212;`, and a ComicInfo or an EXTH header can too. The Códice shows it as text and never as HTML (the file is
not ours to trust), so the markup has to go before it is kept: paragraphs and line breaks stay as line breaks, the
other tags are dropped, and the entities are read.

It is only for what comes from outside. A description the person wrote is never passed through here.
"""
import html
import re

# What starts or ends a block of text (a paragraph, a list), and what ends a line. A tag may carry a namespace
# prefix, as XHTML written inside an EPUB's description does ("<xhtml:p>").
_NS = r'(?:[\w.-]+:)?'
_PARAGRAPH = re.compile(r'<\s*/?\s*' + _NS + r'(?:p|div|h[1-6]|blockquote|section|article|tr|ul|ol)\b[^<>]*>', re.I)
_LINE_END = re.compile(r'<\s*' + _NS + r'br\b[^<>]*>|<\s*/\s*' + _NS + r'li\s*>', re.I)
# What is not text at all: code and styles, with what is inside them; and comments.
_NOT_TEXT = re.compile(r'<\s*(script|style)\b[^>]*>.*?<\s*/\s*\1\s*>|<!--.*?-->', re.I | re.S)
# Something that looks like a tag: a name after the "<" (so "5 < 6" and "<3" are text), or a declaration.
_TAG = re.compile(r'<\s*/?\s*[A-Za-z][^<>]*>|<![A-Za-z][^<>]*>|<\?[^<>]*\?>')
_CONTROL = re.compile(r'[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]')
# What the text still has of markup, for finding what an earlier version stored raw: a tag, or an entity.
MARKUP = r'<[A-Za-z/!]|&(?:#[0-9]+|#[xX][0-9A-Fa-f]+|[A-Za-z][A-Za-z0-9]*);'


def plain_description(text) -> str:
    """The text of a description: tags out, entities read, paragraphs kept, spaces tidied. Text with no markup
    comes back the same, apart from the spaces. Never HTML, whatever went in."""
    if not text:
        return ''
    out = str(text)
    out = _NOT_TEXT.sub(' ', out)
    out = _PARAGRAPH.sub('\n\n', out)
    out = _LINE_END.sub('\n', out)
    out = _TAG.sub('', out)
    # After the tags: "&lt;b&gt;" was written as text and stays text, it is not a tag.
    out = html.unescape(out)
    out = _CONTROL.sub('', out).replace('\r\n', '\n').replace('\r', '\n')
    lines = [re.sub(r'[ \t\f\v ]+', ' ', line).strip() for line in out.split('\n')]
    return re.sub(r'\n{3,}', '\n\n', '\n'.join(lines)).strip()
