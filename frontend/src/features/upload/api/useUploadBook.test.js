import { describe, it, expect } from 'vitest';
import { describeUploadError, isInSeries } from './useUploadBook';

const failure = (status, data) => ({ response: { status, data } });

describe('describeUploadError', () => {
  it('points to the existing record when the bytes are already stored', () => {
    expect(describeUploadError(failure(409, { error: 'duplicate', title: 'Duna.epub', work_id: 4, retired: false })))
      .toBe('Esse arquivo já está no acervo como “Duna.epub”.');
    expect(describeUploadError(failure(409, { title: 'Duna.epub', retired: true })))
      .toContain('(na lixeira)');
  });

  it('says a number already taken in the series is skipped, and not an error of the file', () => {
    const taken = failure(409, { error: 'in_series', title: 'Jujutsu - Cap. 2', work_id: 9 });
    expect(describeUploadError(taken)).toBe('Já existe na série como “Jujutsu - Cap. 2”: foi pulado.');
    expect(isInSeries(taken)).toBe(true);
    expect(isInSeries(failure(409, { error: 'duplicate' }))).toBe(false);
    expect(isInSeries(new Error('x'))).toBe(false);
  });

  it('relays what the server says about content, size and permission', () => {
    expect(describeUploadError(failure(415, 'the content is not a valid .epub file (wrong mimetype)\n')))
      .toBe('the content is not a valid .epub file (wrong mimetype)');
    expect(describeUploadError(failure(413, 'File is too large'))).toBe('O arquivo é grande demais.');
    expect(describeUploadError(failure(403, 'Forbidden'))).toBe('Só quem administra o acervo pode adicionar arquivos.');
    expect(describeUploadError(failure(400, 'Unsupported file format\n'))).toBe('Unsupported file format');
  });

  it('says in its own words that the content is not valid when the server does not say why', () => {
    expect(describeUploadError(failure(415, { error: 'x' }))).toBe('O conteúdo do arquivo não é válido para o formato dele.');
    expect(describeUploadError(failure(400, { error: 'x' }))).toBe('Não foi possível enviar o arquivo.');
    expect(describeUploadError(failure(409, 'text'))).toBe('Não foi possível enviar o arquivo.');
  });

  it('distinguishes a timeout and an unreachable server from a rejection', () => {
    expect(describeUploadError({ code: 'ECONNABORTED' })).toBe('O envio demorou demais e foi interrompido.');
    expect(describeUploadError({})).toBe('Não foi possível falar com o servidor.');
    expect(describeUploadError(failure(500, 'boom'))).toBe('Não foi possível enviar o arquivo.');
  });
});
