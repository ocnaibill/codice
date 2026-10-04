/**
 * What the Códice carries that others made, with the license each one gives it (DEC-120). Not the whole tree of
 * dependencies, which is in go.mod, requirements.txt and package-lock.json of the source: the pieces a person
 * can see or that the license asks to be named. `package` is the npm name when there is one: a test keeps the
 * list in step with package.json, so a font that is added to the app cannot go unnamed here.
 */
export const THIRD_PARTY = [
  { name: 'Newsreader', what: 'fonte dos títulos e do texto', license: 'SIL OFL 1.1', url: 'https://fonts.google.com/specimen/Newsreader', package: '@fontsource-variable/newsreader' },
  { name: 'Plus Jakarta Sans', what: 'fonte da interface', license: 'SIL OFL 1.1', url: 'https://fonts.google.com/specimen/Plus+Jakarta+Sans', package: '@fontsource-variable/plus-jakarta-sans' },
  { name: 'JetBrains Mono', what: 'fonte de códigos e senhas', license: 'SIL OFL 1.1', url: 'https://www.jetbrains.com/lp/mono/', package: '@fontsource-variable/jetbrains-mono' },
  { name: 'OpenDyslexic', what: 'fonte para dislexia', license: 'SIL OFL 1.1', url: 'https://opendyslexic.org', package: '@fontsource/opendyslexic' },
  { name: 'React', what: 'a interface', license: 'MIT', url: 'https://react.dev', package: 'react' },
  { name: 'epub.js', what: 'leitor de EPUB', license: 'BSD-2-Clause', url: 'https://github.com/futurepress/epub.js', package: 'epubjs' },
  { name: 'PDF.js', what: 'leitor de PDF', license: 'Apache-2.0', url: 'https://mozilla.github.io/pdf.js/', package: 'react-pdf' },
  { name: 'Tesseract', what: 'reconhecimento de texto em imagens (OCR)', license: 'Apache-2.0', url: 'https://github.com/tesseract-ocr/tesseract' },
  { name: 'PyMuPDF', what: 'leitura de PDF no servidor', license: 'AGPL-3.0', url: 'https://github.com/pymupdf/PyMuPDF' },
  { name: 'LaBSE', what: 'modelo opcional de busca por sentido', license: 'Apache-2.0', url: 'https://huggingface.co/sentence-transformers/LaBSE' },
  { name: 'Wikcionário', what: 'dicionários, se o dono instalar (dados do Wiktextract, em kaikki.org)', license: 'CC BY-SA 4.0 e GFDL', url: 'https://kaikki.org' },
];
