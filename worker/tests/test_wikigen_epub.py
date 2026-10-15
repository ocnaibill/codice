"""The EPUB as chapters for the generator: spine order, titles and parts from the table of contents, epigraphs, what is left out."""
import zipfile

from wikigen import epub

LONG = 'Uma frase comprida do capítulo, que se repete para passar do mínimo de texto de um capítulo de verdade. ' * 8

CONTAINER = '<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles>' \
            '<rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>'


def page(title, *paras):
    body = ''.join(f'<p>{p}</p>' for p in paras)
    return f'<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>{title}</h1>{body}</body></html>'


def make(tmp_path, files, toc):
    items = ''.join(f'<item id="i{n}" href="{name}" media-type="application/xhtml+xml"/>' for n, name in enumerate(files))
    refs = ''.join(f'<itemref idref="i{n}"/>' for n in range(len(files)))
    opf = ('<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="2.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">'
           f'<dc:title>O Livro</dc:title></metadata><manifest>{items}'
           '<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/></manifest>'
           f'<spine toc="ncx">{refs}</spine></package>')
    points = ''
    for name, label in toc:
        points += f'<navPoint><navLabel><text>{label}</text></navLabel><content src="{name}"/></navPoint>'
    ncx = f'<?xml version="1.0"?><ncx xmlns="http://www.daisy.org/z3986/2005/ncx/"><navMap>{points}</navMap></ncx>'
    path = tmp_path / 'livro.epub'
    with zipfile.ZipFile(path, 'w') as z:
        z.writestr('META-INF/container.xml', CONTAINER)
        z.writestr('OEBPS/content.opf', opf)
        z.writestr('OEBPS/toc.ncx', ncx)
        for name, markup in files.items():
            z.writestr(f'OEBPS/{name}', markup)
    return str(path)


def test_chapters_come_in_spine_order_with_title_part_and_href(tmp_path):
    files = {
        'capa.xhtml': page('Capa', 'curta'),
        'p1.xhtml': page('Parte I', 'x' * 450),
        'c1.xhtml': page('Um', LONG, 'Segundo parágrafo.'),
        'c2.xhtml': page('Dois', LONG),
        'p2.xhtml': page('Parte II', 'y' * 450),
        'c3.xhtml': page('Três', LONG),
    }
    toc = [('capa.xhtml', 'Capa'), ('p1.xhtml', 'Parte I'), ('c1.xhtml', '1. Um'), ('c2.xhtml', '2. Dois'), ('p2.xhtml', 'Parte II'), ('c3.xhtml', '3. Três')]
    book = epub.read(make(tmp_path, files, toc))
    assert book['title'] == 'O Livro'
    got = [(c['index'], c['title'], c['href']) for c in book['chapters']]
    assert got == [(1, 'Parte I', 'OEBPS/p1.xhtml'), (2, '1. Um', 'OEBPS/c1.xhtml'), (3, '2. Dois', 'OEBPS/c2.xhtml'),
                   (4, 'Parte II', 'OEBPS/p2.xhtml'), (5, '3. Três', 'OEBPS/c3.xhtml')]
    assert book['chapters'][1]['paragraphs'][0] == 'Um'     # the heading is the first paragraph
    assert book['chapters'][1]['paragraphs'][2] == 'Segundo parágrafo.'
    assert 'capítulo' in book['chapters'][1]['paragraphs'][1]   # accents survive: the parser is told the encoding
    assert all('Capa' != c['title'] for c in book['chapters'])


def test_a_short_file_the_table_does_not_cite_is_the_epigraph_of_the_next_chapter(tmp_path):
    files = {'ep.xhtml': page('', 'Uma epígrafe longa o bastante para contar como texto. ' * 10), 'c1.xhtml': page('Um', LONG)}
    book = epub.read(make(tmp_path, files, [('c1.xhtml', '1. Um')]))
    assert len(book['chapters']) == 1
    ch = book['chapters'][0]
    assert ch['title'] == '1. Um' and ch['epigraph_paragraphs'] == 1 and ch['href'] == 'OEBPS/ep.xhtml'
    assert any(p.startswith('Uma epígrafe') for p in ch['paragraphs'])


def test_credits_and_pages_with_almost_no_text_are_not_chapters(tmp_path):
    files = {'c1.xhtml': page('Um', LONG), 'cred.xhtml': page('Créditos', LONG), 'vazia.xhtml': page('Nada', 'oi')}
    toc = [('c1.xhtml', '1. Um'), ('cred.xhtml', 'Créditos'), ('vazia.xhtml', 'Vazia')]
    assert [c['title'] for c in epub.read(make(tmp_path, files, toc))['chapters']] == ['1. Um']


def test_soft_hyphens_are_taken_out_of_the_text(tmp_path):
    files = {'c1.xhtml': page('Um', 'cor­ti­na ' * 80)}
    ch = epub.read(make(tmp_path, files, [('c1.xhtml', '1. Um')]))['chapters'][0]
    assert any(p.startswith('cortina cortina') for p in ch['paragraphs'])
