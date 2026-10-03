# dictionary-sample.jsonl.gz

A few dozen entries copied as they are from `pt-extract.jsonl` (the Portuguese Wiktionary extracted by Wiktextract,
published at https://kaikki.org/dictionary/rawdata.html, extraction of 28 September 2026), to test the importer
(`worker/dictionary.py`) on real data: a lemma and its inflected form (`correr`, `correram`), homographs (`livro`),
words of English, Spanish, French, Italian, German, Japanese and Chinese (simplified and traditional), one language the
importer does not keep (`gl`, `yue`), a verb whose forms include a dash, and two synthetic lines that the real file cannot
give (an entry with no sense, and a line that is not JSON).

The content is from the Wiktionary and is licensed CC BY-SA 4.0 and GFDL; it is credited to its authors (the history of
each page at pt.wiktionary.org). It is test data: the application does not ship a dictionary.
