import { readdirSync, readFileSync } from 'node:fs';
import { describe, it, expect } from 'vitest';
import { messageOf, serverMessage } from './serverMessage';

const refused = (status, data) => ({ response: { status, data } });

describe('messageOf', () => {
  it('says in Portuguese what the server said in English, for what a person can bring about', () => {
    expect(messageOf('Invalid username or password')).toBe('Usuário ou senha incorretos.');
    expect(messageOf('The new password must have at least 8 characters')).toBe('A senha precisa ter pelo menos 8 caracteres.');
    expect(messageOf('Note not found')).toBe('Anotação não encontrada.');
    expect(messageOf('The path must be absolute')).toBe('O caminho precisa ser absoluto (começar por /).');
  });

  it('says what has a number in it with the number', () => {
    expect(messageOf('The note is longer than 20000 characters')).toBe('A anotação passa de 20000 caracteres.');
    expect(messageOf('a tag has at most 40 characters')).toBe('Uma tag tem no máximo 40 caracteres.');
    expect(messageOf('at most 20 tags')).toBe('No máximo 20 tags.');
    expect(messageOf('invalid locator: page 0 is not a page')).toBe('O lugar indicado não existe neste arquivo.');
    expect(messageOf('text is not valid UTF-8')).toBe('O texto tem caracteres que não dá para guardar.');
  });

  it('keeps what the server already said in Portuguese, as it came', () => {
    for (const text of ['Informe a palavra e o idioma.', 'Cancele a instalação antes de remover.', 'O OCR está desligado.', 'Dicionário não encontrado', 'Nenhuma palavra para procurar.', 'Você chegou ao limite de conceitos.']) {
      expect(messageOf(text), text).toBe(text);
    }
  });

  it('says nothing of what is in English and has not been translated: the screen says its own', () => {
    for (const text of ['ids is required (1 to 500 job ids)', 'Pages not yet extracted. Call GetPages first.', 'Format does not support page listing', 'some new thing is not allowed']) {
      expect(messageOf(text), text).toBe('');
    }
  });

  it('trims it, and has nothing for nothing or for what is not text', () => {
    expect(messageOf('  Note not found \n')).toBe('Anotação não encontrada.');
    for (const value of ['', '   ', null, undefined, 42, { a: 1 }, ['x']]) expect(messageOf(value)).toBe('');
  });
});

describe('serverMessage', () => {
  it('is what the server said, in Portuguese, for a refusal', () => {
    expect(serverMessage(refused(401, 'Invalid username or password'), 'Falha.')).toBe('Usuário ou senha incorretos.');
    expect(serverMessage(refused(400, 'Informe a palavra e o idioma.'), 'Falha.')).toBe('Informe a palavra e o idioma.');
  });

  it('is the screen\'s own sentence when the server said nothing a person can use', () => {
    expect(serverMessage(refused(400, 'ids are numbers'), 'Não foi possível salvar.')).toBe('Não foi possível salvar.');
    expect(serverMessage(refused(400, ''), 'Não foi possível salvar.')).toBe('Não foi possível salvar.');
    expect(serverMessage(refused(400, { error: 'x' }), 'Não foi possível salvar.')).toBe('Não foi possível salvar.');
  });

  it('never reads out a failure of the server itself, even in Portuguese', () => {
    expect(serverMessage(refused(500, 'Error changing the password'), 'Não foi possível trocar a senha.')).toBe('Não foi possível trocar a senha.');
    expect(serverMessage(refused(503, 'Informe algo.'), 'Fora do ar.')).toBe('Fora do ar.');
    expect(serverMessage(refused(499, 'Note not found'), 'x')).toBe('Anotação não encontrada.');
  });

  it('is the screen\'s own sentence when the server did not answer at all', () => {
    expect(serverMessage(new Error('Network Error'), 'Sem resposta.')).toBe('Sem resposta.');
    expect(serverMessage(undefined, 'Sem resposta.')).toBe('Sem resposta.');
  });
});

// What the server can say is read from its source, so that a message written in English tomorrow is not shown to a person as it is:
// it is translated here, or it is said to be for programs only.
describe('what the server says', () => {
  const DIR = '../backend/internal/handlers';
  const literals = readdirSync(DIR)
    .filter((name) => name.endsWith('.go') && !name.endsWith('_test.go'))
    .flatMap((name) => [...readFileSync(`${DIR}/${name}`, 'utf8').matchAll(/http\.Error\(w, "((?:[^"\\]|\\.)+)", http\.Status(?:BadRequest|Unauthorized|Forbidden|NotFound|Conflict|Gone|Unprocessable\w*|TooMany\w*|ServiceUnavailable|RequestEntityTooLarge|UnsupportedMediaType)\)/g)].map((m) => m[1].replace(/\\"/g, '"')));
  // ... and what the packages of the server refuse with, which the handlers read out as it is (var ErrX = errors.New("..."))
  const SRC = '../backend/internal';
  const walk = (dir) => readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(`${dir}/${e.name}`) : [`${dir}/${e.name}`]));
  const sentinels = walk(SRC)
    .filter((file) => file.endsWith('.go') && !file.endsWith('_test.go'))
    .flatMap((file) => [...readFileSync(file, 'utf8').matchAll(/\bErr\w+\s*=\s*errors\.New\("([^"]+)"\)/g)].map((m) => m[1]));
  const unique = [...new Set([...literals, ...sentinels])].sort();

  // Messages for programs that call the API, which no screen of the app brings about: they are not translated.
  const FOR_PROGRAMS = [
    // the filters and the setting of the record of sign-ins, which the screen sends from lists and a field it checks first
    'The page is not valid',
    'The period is not valid: use RFC 3339 dates',
    'retentionDays is required',
    'retentionDays must be between 7 and 3650',
    // the choices of the reader, which the app sends from the lists it offers
    'reader is a choice of the lists of the reader',
    'the reading preferences are not valid',
    // the command line of the backup, which a person runs on the server and reads in the terminal
    'other connections are using the target database: stop the API and the worker first',
    'pg_dump and pg_restore are needed (install the PostgreSQL client tools)',
    'pg_dump is newer than the database server, so the package might not restore on it',
    'the package is corrupt or incomplete',
    'the package is encrypted: give the passphrase (CODICE_BACKUP_PASSPHRASE or --passphrase-file)',
    'the package was made on a newer PostgreSQL than the target server runs',
    'the passphrase does not open this package',
    'the target database already holds data',
    'the word reads as nothing',
    'Deleting for good needs confirm=true',
    'Emptying the trash needs confirm=true',
    'Expected a multipart form',
    'display name is too long', // never reaches a client: the preferences answer with the line below
    'displayName is at most 60 characters', // the screen does not let a longer name be typed
    'Format does not support page listing',
    'Format does not support page serving',
    'Invalid JSON payload',
    'Invalid multipart form',
    'not UTF-8, and not Windows-1252 or UTF-16 with a byte order mark', // never reaches a client: the upload answers with the filecheck message
    'Invalid page number',
    'Pages not yet extracted. Call GetPages first.',
    'The hash of the previewed plan is required',
    'completed is true or false; restart only goes with false',
    'days and confirm=true are required; preview first',
    'enabled must be true or false',
    'fileIds is required (1 to 500 ids)',
    'finished is true or false',
    'format is md or json',
    'from is the file id you are reading now',
    'from must be a different file',
    'ids are numbers',
    'ids is required (1 to 500 job ids)',
    'into is the work that stays',
    'invalid acceptance',
    'invalid concept',
    'invalid language',
    'invalid relation',
    'invalid settings',
    'keep (the person who stays) and confirm=true are required',
    'keep (the work that stays) and confirm=true are required',
    'kind is note, highlight or bookmark',
    'nameOrder is given_first or family_first',
    'nameOrder is given_first, family_first or empty',
    'paths is required (1 to 1000)',
    'percent is not a number',
    'q is what to search for',
    'rootId is a number',
    'rootId is required',
    "scope must be 'assets' or 'ws'",
    'state is ok, missing or conflict',
    'targetId and formerRole are required',
    'workId is the other work',
  ];

  it('is read from the source of the server', () => {
    expect(unique.length).toBeGreaterThan(100);
  });

  it('is said in Portuguese when a person can bring it about, and listed here when only a program can', () => {
    const left = unique.filter((text) => !messageOf(text) && !FOR_PROGRAMS.includes(text));
    expect(left).toEqual([]);
  });

  it('is not listed as for programs when it is already said in Portuguese', () => {
    expect(FOR_PROGRAMS.filter((text) => messageOf(text))).toEqual([]);
  });

  it('is only listed as for programs when the server still says it', () => {
    expect(FOR_PROGRAMS.filter((text) => !unique.includes(text))).toEqual([]);
  });
});
