import { describe, it, expect } from 'vitest';
import { coverageNote, fileTextState } from './processing';

describe('fileTextState', () => {
  it('says the text is indexed, when there is text', () => {
    expect(fileTextState({ textStatus: 'ready', textSegments: 40 })).toMatchObject({ text: 'texto indexado', tone: 'ok' });
  });

  it('says there is nothing to find when it was read and there is none', () => {
    expect(fileTextState({ textStatus: 'ready', textSegments: 0 })).toMatchObject({ text: 'nenhum texto pesquisável', tone: 'warn' });
    expect(fileTextState({ textStatus: 'ready' })).toMatchObject({ text: 'nenhum texto pesquisável' });
  });

  it('says the text could not be read, and that the file still opens', () => {
    const state = fileTextState({ textStatus: 'failed' });
    expect(state).toMatchObject({ text: 'texto não lido', tone: 'warn' });
    expect(state.title).toContain('O arquivo abre');
  });

  it('says a file with no text has none, and leaves a scan to the note of the OCR', () => {
    expect(fileTextState({ textStatus: 'empty' })).toMatchObject({ text: 'sem texto pesquisável', tone: 'warn' });
    expect(fileTextState({ textStatus: 'empty', needsOcr: true }, { text: 'Páginas sem texto' })).toBeNull();
  });

  it('has nothing to say of a kind of file that has no text', () => {
    expect(fileTextState({ textStatus: 'unsupported' })).toBeNull();
  });

  it('says nothing of text waiting for a comic or an audiobook, which are not read for text', () => {
    for (const format of ['mp3', 'cbz', 'CBR', 'm4b']) expect(fileTextState({ format, textStatus: '' }), format).toBeNull();
    expect(fileTextState({ format: 'EPUB', textStatus: '' })).toMatchObject({ text: 'texto na fila' });
    for (const format of ['pdf', 'epub', 'txt', 'md', 'mobi']) expect(fileTextState({ format }), format).toMatchObject({ text: 'texto na fila' });
  });

  it('says the text is waiting when the server has not said yet, and that the file can be opened', () => {
    for (const file of [{}, { textStatus: '' }, { textStatus: undefined }]) {
      const state = fileTextState(file);
      expect(state).toMatchObject({ text: 'texto na fila', tone: 'plain' });
      expect(state.title).toContain('já pode ser aberto');
    }
    expect(fileTextState(null)).toMatchObject({ text: 'texto na fila' });
  });
});

describe('coverageNote', () => {
  it('has nothing to say when the search sees everything, or the server does not say', () => {
    expect(coverageNote({ reading: 0, failed: 0, noText: 0 })).toBeNull();
    expect(coverageNote({})).toBeNull();
    expect(coverageNote(null)).toBeNull();
    expect(coverageNote(undefined)).toBeNull();
  });

  it('says what is still being read', () => {
    expect(coverageNote({ reading: 3 })).toBe('A busca ainda não vê tudo: o texto de 3 arquivos ainda está sendo lido. Uma passagem que não aparece pode estar num desses arquivos.');
    expect(coverageNote({ reading: 1 })).toContain('o texto de 1 arquivo ainda está sendo lido');
  });

  it('says what could not be read, and what has no text', () => {
    expect(coverageNote({ failed: 1 })).toContain('o texto de 1 arquivo não pôde ser lido');
    expect(coverageNote({ failed: 2 })).toContain('o texto de 2 arquivos não pôde ser lido');
    expect(coverageNote({ noText: 1 })).toContain('1 arquivo não tem texto (digitalizações, que o OCR pode ler)');
    expect(coverageNote({ noText: 4 })).toContain('4 arquivos não têm texto (digitalizações, que o OCR pode ler)');
  });

  it('says all of it in one sentence, with commas and an "e"', () => {
    const note = coverageNote({ reading: 2, failed: 1, noText: 3 });
    expect(note).toBe('A busca ainda não vê tudo: o texto de 2 arquivos ainda está sendo lido, o texto de 1 arquivo não pôde ser lido e 3 arquivos não têm texto (digitalizações, que o OCR pode ler). Uma passagem que não aparece pode estar num desses arquivos.');
    expect(coverageNote({ reading: 2, noText: 3 })).toContain('lido e 3 arquivos não têm');
  });
});

describe('fileTextState: a PDF that asks for a password (#89)', () => {
  it('says so, whatever the text status says, and warns', () => {
    for (const textStatus of ['failed', 'ready', 'empty', undefined]) {
      const state = fileTextState({ format: 'pdf', protected: true, textStatus, textSegments: 3 });
      expect(state.text, String(textStatus)).toBe('PDF com senha');
      expect(state.tone).toBe('warn');
      expect(state.title).toContain('abre no leitor, com a senha');
    }
  });

  it('is not said of a file that does not ask for it', () => {
    expect(fileTextState({ format: 'pdf', protected: false, textStatus: 'failed' }).text).toBe('texto não lido');
    expect(fileTextState({ format: 'pdf', textStatus: 'ready', textSegments: 4 }).text).toBe('texto indexado');
  });
});
