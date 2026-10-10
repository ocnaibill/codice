import { describe, it, expect } from 'vitest';
import { describeHealth, TONE_CLASS } from './providerHealth';

const NOW = new Date('2026-10-10T12:00:00Z').getTime();
const minutes = (n) => new Date(NOW - n * 60000).toISOString();
const provider = (over = {}, health = undefined) => ({ id: 'openlibrary', name: 'Open Library', enabled: true, health: health === undefined ? null : health, ...over });
const health = (over = {}) => ({ state: 'ok', status: 200, problem: '', checkedAt: minutes(3), lastOkAt: minutes(3), empty: false, ...over });

describe('describeHealth: how a provider answered the last time (DEC-144)', () => {
  it('says nothing of a provider that is off and was never asked, and says it was not asked yet when it is on', () => {
    expect(describeHealth(provider({ enabled: false }), NOW)).toBeNull();
    expect(describeHealth(provider({ enabled: true }), NOW)).toEqual({ tone: 'faint', text: 'Ainda não foi perguntado: nada a dizer sobre ele.' });
  });

  it('says it is answering, and when it was last asked', () => {
    expect(describeHealth(provider({}, health()), NOW)).toEqual({ tone: 'ok', text: 'Respondendo normalmente (último pedido há 3 minutos).' });
    expect(describeHealth(provider({}, health({ checkedAt: minutes(0) })), NOW).text).toBe('Respondendo normalmente (último pedido agora há pouco).');
    expect(describeHealth(provider({}, health({ checkedAt: minutes(60 * 5) })), NOW).text).toBe('Respondendo normalmente (último pedido há 5 horas).');
    expect(describeHealth(provider({}, health({ checkedAt: minutes(60 * 24 * 3) })), NOW).text).toBe('Respondendo normalmente (último pedido há 3 dias).');
  });

  it('warns when many searches in a row came back with nothing though every request was answered', () => {
    const got = describeHealth(provider({}, health({ empty: true })), NOW);
    expect(got.tone).toBe('warn');
    expect(got.text).toBe('Respondeu, mas sem nada nas últimas buscas seguidas (último pedido há 3 minutos). Se as obras existem, confira a chave e a conexão.');
  });

  it('says a key was refused, where to look for it, and when it last worked', () => {
    const got = describeHealth(provider({ id: 'google_books', name: 'Google Books' }, health({ state: 'key', status: 403, checkedAt: minutes(5), lastOkAt: minutes(60 * 30) })), NOW);
    expect(got.tone).toBe('danger');
    expect(got.text).toBe('A chave foi recusada (HTTP 403, último pedido há 5 minutos). Confira GOOGLE_BOOKS_API_KEY no ambiente do worker e reinicie-o. Funcionou pela última vez há 30 horas.');
    expect(describeHealth(provider({ id: 'comicvine' }, health({ state: 'key', status: 401 })), NOW).text).toContain('Confira COMICVINE_API_KEY');
    expect(describeHealth(provider({ id: 'openlibrary' }, health({ state: 'key', status: 401 })), NOW).text).toContain('Confira o acesso a este serviço.');
  });

  it('says the quota is used up, with a way out', () => {
    const got = describeHealth(provider({}, health({ state: 'quota', status: 429 })), NOW);
    expect(got.tone).toBe('warn');
    expect(got.text).toBe('O limite de uso foi atingido (HTTP 429, último pedido há 3 minutos). Espere, ou use uma chave com cota maior. Funcionou pela última vez há 3 minutos.');
  });

  it('says the service did not answer, and an error it answered with', () => {
    const down = describeHealth(provider({}, health({ state: 'down', status: 0, lastOkAt: null })), NOW);
    expect(down).toEqual({ tone: 'danger', text: 'Não respondeu: rede ou serviço fora do ar (último pedido há 3 minutos). Ainda não respondeu bem.' });
    const error = describeHealth(provider({}, health({ state: 'error', status: 400 })), NOW);
    expect(error.tone).toBe('warn');
    expect(error.text).toBe('Respondeu com erro (HTTP 400, último pedido há 3 minutos). Funcionou pela última vez há 3 minutos.');
  });

  it('says it for a provider that was turned off after it was asked, because it is worth knowing before turning it back on', () => {
    expect(describeHealth(provider({ enabled: false }, health({ state: 'down', status: 0 })), NOW).tone).toBe('danger');
  });

  it('has a color for every tone', () => {
    expect(TONE_CLASS).toEqual({ ok: 'text-success', warn: 'text-warning', danger: 'text-danger', faint: 'text-ink-faint' });
  });
});
