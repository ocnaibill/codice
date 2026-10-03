import { describe, it, expect } from 'vitest';
import { describeError } from './admin';

const refused = (status, data) => ({ response: { status, data } });

describe('describeError', () => {
  it('says what the server said, in Portuguese, for a refusal', () => {
    expect(describeError(refused(403, 'Forbidden\n'))).toBe('Você não tem permissão para isso.');
    expect(describeError(refused(409, 'The job is not in a state that allows this'))).toBe('Este trabalho não está num estado que permita isso.');
    expect(describeError(refused(400, 'Informe a palavra e o idioma.'))).toBe('Informe a palavra e o idioma.');
  });

  it('says nothing of a failure of the server itself, even when it said something a person could read', () => {
    expect(describeError(refused(500, 'Note not found'))).toBe('Algo deu errado.');
    expect(describeError(refused(502, 'Informe algo.'))).toBe('Algo deu errado.');
  });

  it('does not show English that nobody translated', () => {
    expect(describeError(refused(400, 'ids are numbers'))).toBe('Algo deu errado.');
    expect(describeError(refused(403, 'Not a thing we know is not allowed'))).toBe('Você não tem permissão para isso.');
  });

  it('says it could not talk to the server when there was no answer', () => {
    expect(describeError(new Error('Network Error'))).toBe('Não foi possível falar com o servidor.');
    expect(describeError(undefined)).toBe('Não foi possível falar com o servidor.');
  });
});
