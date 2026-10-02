import { describe, it, expect } from 'vitest';
import { fileProgress, languageLine, languageName, languagesLabel, languageSource, sheetNote } from './ocr';

const item = (over) => ({ pagesWithoutText: [1, 2, 3, 4], read: 0, failed: 0, state: '', languages: [], ...over });

describe('languages', () => {
  it('are named in Portuguese, and a code that is not known is shown as it is', () => {
    expect(languageName('por')).toBe('Português');
    expect(languageName('eng')).toBe('Inglês');
    expect(languageName('xyz')).toBe('xyz');
  });

  it('are listed the way a person would say them', () => {
    expect(languagesLabel('por')).toBe('Português');
    expect(languagesLabel('por+eng')).toBe('Português e Inglês');
    expect(languagesLabel(['por', 'eng', 'spa'])).toBe('Português, Inglês e Espanhol');
    expect(languagesLabel(['por+eng', 'por'])).toBe('Português e Inglês'); // the sets the pages were read with, as the API lists them
    expect(languagesLabel(['por+eng', 'spa'])).toBe('Português, Inglês e Espanhol');
    expect(languagesLabel('')).toBe('');
    expect(languagesLabel(null)).toBe('');
    expect(languagesLabel([])).toBe('');
  });
});

describe('fileProgress (the administration)', () => {
  it('says how many pages are read while it reads, and that the work waits when it is in line', () => {
    expect(fileProgress(item({ state: 'reading', read: 2 }), true)).toEqual({ text: 'Lendo: 2 de 4 páginas', tone: 'plain' });
    expect(fileProgress(item({ state: 'queued' }), true)).toEqual({ text: 'Na fila para ser lido', tone: 'plain' });
  });

  it('says it is done, in what language, and in singular for a single page', () => {
    expect(fileProgress(item({ read: 4, languages: ['por+eng'] }), true)).toEqual({ text: 'As 4 páginas foram lidas (Português e Inglês)', tone: 'ok' });
    expect(fileProgress(item({ pagesWithoutText: [3], read: 1, languages: ['por'] }), true)).toEqual({ text: 'A página foi lida (Português)', tone: 'ok' });
    expect(fileProgress(item({ read: 4 }), true).text).toBe('As 4 páginas foram lidas');
  });

  it('warns of the pages that could not be read, in singular and in plural', () => {
    expect(fileProgress(item({ read: 3, failed: 1, languages: ['por'] }), true)).toEqual({ text: '3 de 4 páginas lidas (Português); 1 não pôde ser lida', tone: 'warn' });
    expect(fileProgress(item({ read: 1, failed: 3 }), true)).toEqual({ text: '1 de 4 páginas lidas; 3 não puderam ser lidas', tone: 'warn' });
  });

  it('says it waits for the engine to be turned on, or for its turn', () => {
    expect(fileProgress(item(), false).text).toBe('4 páginas sem texto, esperando o OCR ser ligado');
    expect(fileProgress(item({ pagesWithoutText: [1] }), false).text).toBe('1 página sem texto, esperando o OCR ser ligado');
    expect(fileProgress(item(), true).text).toBe('4 páginas esperando a vez');
    expect(fileProgress(item({ read: 2 }), true).text).toBe('2 de 4 páginas lidas; esperando a vez');
  });

  it('puts a job that is running before what was already read', () => {
    expect(fileProgress(item({ state: 'reading', read: 1, failed: 1 }), true).text).toBe('Lendo: 1 de 4 páginas');
  });
});

describe('sheetNote (the sheet of a work)', () => {
  const file = (ocr) => ({ needsOcr: true, ocr });

  it('is nothing for a file that has no pages without text', () => {
    expect(sheetNote({ needsOcr: false })).toBeNull();
    expect(sheetNote({})).toBeNull();
    expect(sheetNote(null)).toBeNull();
  });

  it('says what is going on, and that the text is recognised and may have mistakes once it is', () => {
    expect(sheetNote(file({ pages: 300, read: 12, failed: 0, state: 'reading' })).text).toBe('Lendo as páginas sem texto (12 de 300)');
    expect(sheetNote(file({ pages: 300, read: 0, failed: 0, state: 'queued' })).text).toBe('Páginas sem texto na fila para leitura');
    const done = sheetNote(file({ pages: 300, read: 300, failed: 0 }));
    expect(done).toMatchObject({ text: 'Texto reconhecido por OCR', tone: 'ok' });
    expect(done.title).toContain('pode ter erros');
  });

  it('warns when some pages failed, and when nothing was read', () => {
    const failed = sheetNote(file({ pages: 10, read: 7, failed: 3 }));
    expect(failed).toMatchObject({ text: 'Texto reconhecido por OCR em 7 de 10 páginas; 3 falharam', tone: 'warn' });
    expect(sheetNote(file({ pages: 10, read: 0, failed: 0 }))).toMatchObject({ text: 'Páginas sem texto', tone: 'warn' });
    expect(sheetNote(file(undefined))).toMatchObject({ text: 'Páginas sem texto', tone: 'warn' });
  });
});

describe('languageLine and languageSource', () => {
  it('say how the language of a file was chosen', () => {
    expect(languageSource('declared')).toBe('o arquivo declara');
    expect(languageSource('detected')).toBe('descoberto pelo Códice lendo algumas páginas');
    expect(languageSource('default')).toBe('não deu para descobrir, é o padrão do dono do acervo');
    expect(languageSource('manual')).toBe('escolhido pela equipe');
    expect(languageSource('guess')).toBe('');
    expect(languageSource(undefined)).toBe('');
  });

  it('is a line for a file that has been looked at, and nothing before', () => {
    expect(languageLine({ language: 'por+eng', languageSource: 'default' })).toBe('Idioma da leitura: Português e Inglês (não deu para descobrir, é o padrão do dono do acervo)');
    expect(languageLine({ language: 'eng', languageSource: 'manual' })).toBe('Idioma da leitura: Inglês (escolhido pela equipe)');
    expect(languageLine({ language: 'eng' })).toBe('Idioma da leitura: Inglês');
    expect(languageLine({})).toBe('');
    expect(languageLine({ language: '' })).toBe('');
    expect(languageLine(null)).toBe('');
  });
});
