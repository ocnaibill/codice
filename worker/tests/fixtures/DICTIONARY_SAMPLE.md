# dictionary-sample.jsonl.gz

A few dozen entries copied as they are from `pt-extract.jsonl` (the Portuguese Wiktionary extracted by Wiktextract,
published at https://kaikki.org/dictionary/rawdata.html, extraction of 28 September 2026), to test the importer
(`worker/dictionary.py`) on real data: a lemma and its inflected form (`correr`, `correram`), homographs (`livro`),
words of English, Spanish, French, Italian, German, Japanese and Chinese (simplified and traditional), one language the
importer does not keep (`gl`, `yue`), a verb whose forms include a dash, and two synthetic lines that the real file cannot
give (an entry with no sense, and a line that is not JSON).

The content is from the Wiktionary and is licensed CC BY-SA 4.0 and GFDL; it is credited to its authors (the history of
each page at pt.wiktionary.org). It is test data: the application does not ship a dictionary.

# dictionary-sample-ja.jsonl.gz and dictionary-sample-it.jsonl.gz

The same kind of sample from the Japanese and the Italian Wiktionaries (`ja-extract.jsonl` and `it-extract.jsonl`,
extraction of 28 September 2026): a few dozen real entries each, with a headword in the edition's own language (a kanji that
is the written form of another word, a form of a verb, a lemma with translations), and a few words of other languages that
the importer does not keep when the package keeps only the edition's language. Same license, same use: test data.

# dictionary-sample-en.jsonl.gz

A dozen real entries from the English Wiktionary by language of the words (`kaikki.org-dictionary-English.jsonl`, the
file of English words, taken from its first 100 MB on 3 October 2026): English words with their translations into many
languages (`book`, `barter`, `today`, `policy`, `increase`, `accurate`, `mobile`, `bomb`), inflected forms that point to their
lemma (`pies`, `wares`, `free`) and words with no translation. They test the bridge through English: what the importer keeps
as translations and as links. Same license (CC BY-SA 4.0 and GFDL), same use: test data.
