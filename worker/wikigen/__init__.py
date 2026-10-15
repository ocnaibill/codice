"""The automatic wiki of a work or a series (DEC-177 to DEC-186): the generator.

It reads the text of the books the library already extracted, asks the provider the owner configured, and hands the backend one
bundle that its importer stores (backend/internal/wiki). The stages, in order:

  text      the EPUB of each book becomes chapters and paragraphs
  map       one request per chapter: who and what it shows, with a quote that the program checks in the text
  identity  the entities of every book become one registry (rules first, then the model, only with proof from the notes)
  pages     for each entity with enough chapters: sections written once from numbered facts, then checked sentence by sentence
  import    the backend stores the bundle

Every request to the model is kept by the hash of what was asked, so a run that stops, a book that comes in, or a lower cut for full
pages only pays for what is new.
"""
