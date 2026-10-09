# Test corpus: what the pipeline would send (the title the file brings, cleaned), what the file also says,
# and what a correct answer is. `kind`: book | comic | manga.
CASES = [
 # books, English
 dict(id='dune', kind='book', sent='Dune', author='Frank Herbert', want=['dune'], want_author=['herbert']),
 dict(id='neuromancer', kind='book', sent='Neuromancer', author='William Gibson', want=['neuromancer'], want_author=['gibson']),
 dict(id='pragprog', kind='book', sent='The Pragmatic Programmer', author='Andrew Hunt', want=['pragmatic programmer'], want_author=['hunt', 'thomas']),
 dict(id='modernos', kind='book', sent='Modern Operating Systems', author='Andrew S. Tanenbaum', want=['modern operating systems'], want_author=['tanenbaum']),
 dict(id='cleancode', kind='book', sent='Clean Code', author='Robert C. Martin', want=['clean code'], want_author=['martin']),
 dict(id='sicp', kind='book', sent='Structure and Interpretation of Computer Programs', author='Harold Abelson', want=['structure and interpretation'], want_author=['abelson', 'sussman']),
 dict(id='1984', kind='book', sent='1984', author='George Orwell', want=['1984', 'nineteen eighty'], want_author=['orwell']),
 dict(id='hobbit', kind='book', sent='The Hobbit', author='J. R. R. Tolkien', want=['hobbit'], want_author=['tolkien']),
 # books, Portuguese
 dict(id='duna_pt', kind='book', sent='Duna', author='Frank Herbert', want=['duna', 'dune'], want_author=['herbert']),
 dict(id='casmurro', kind='book', sent='Dom Casmurro', author='Machado de Assis', want=['dom casmurro'], want_author=['assis']),
 dict(id='alquimista', kind='book', sent='O Alquimista', author='Paulo Coelho', want=['alquimista', 'alchemist'], want_author=['coelho']),
 dict(id='sapiens', kind='book', sent='Sapiens: Uma breve história da humanidade', author='Yuval Noah Harari', want=['sapiens'], want_author=['harari']),
 dict(id='lusiadas', kind='book', sent='Os Lusíadas', author='Luís de Camões', want=['lus'], want_author=['cam']),
 dict(id='hp_pt', kind='book', sent='Harry Potter e a Pedra Filosofal', author='J. K. Rowling', want=['harry potter'], want_author=['rowling']),
 # a title polluted with the author, as file names are (the example the owner gave)
 dict(id='nuvem', kind='book', sent='A Nuvem 2 - Neal Shusterman', author='Neal Shusterman', want=['thunderhead', 'nuvem'], want_author=['shusterman']),
 dict(id='scythe_pt', kind='book', sent='Scythe - Neal Shusterman', author='Neal Shusterman', want=['scythe', 'ceifador'], want_author=['shusterman']),
 # comics (western): the title is the file name, with the issue number
 dict(id='absbatman6', kind='comic', sent='Absolute Batman 006', author=None, want=['absolute batman'], want_author=[], issue=6),
 dict(id='saga1', kind='comic', sent='Saga 001', author=None, want=['saga'], want_author=['vaughan', 'staples'], issue=1),
 dict(id='watchmen1', kind='comic', sent='Watchmen 01', author=None, want=['watchmen'], want_author=['moore', 'gibbons'], issue=1),
 dict(id='sandman1', kind='comic', sent='The Sandman 001', author=None, want=['sandman'], want_author=['gaiman'], issue=1),
 # manga: the title is the file name, with the volume
 dict(id='berserk1', kind='manga', sent='Berserk v01', author=None, want=['berserk'], want_author=['miura'], issue=1),
 dict(id='vagabond1', kind='manga', sent='Vagabond 01', author=None, want=['vagabond'], want_author=['inoue'], issue=1),
 dict(id='naruto1', kind='manga', sent='Naruto v01', author=None, want=['naruto'], want_author=['kishimoto'], issue=1),
 dict(id='onepiece1', kind='manga', sent='One Piece 001', author=None, want=['one piece'], want_author=['oda'], issue=1),
 dict(id='chainsaw1', kind='manga', sent='Chainsaw Man 01', author=None, want=['chainsaw man'], want_author=['fujimoto']),
]
