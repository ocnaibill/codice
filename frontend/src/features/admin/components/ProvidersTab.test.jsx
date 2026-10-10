import { act } from 'react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), put: vi.fn(), post: vi.fn() } }));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { ProvidersTab } from './ProvidersTab';
import { pollWhileTesting } from '../api/admin';
import { POLL_MS } from '../systemLimits';

const provider = (id, name, enabled, over = {}) => ({ id, name, enabled, sends: ['title'], key: '', keyConfigured: null, ...over });
const providers = [
  provider('google_books', 'Google Books', false, { key: 'required', sends: ['title', 'isbn'] }),
  provider('openlibrary', 'Open Library', true, { sends: ['title', 'isbn', 'author_key'] }),
  provider('comicvine', 'ComicVine', false, { key: 'required', keyConfigured: true }),
];

let view;
async function open(props = { isOwner: true }, data = providers) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/metadata-providers') {
      if (data instanceof Error) throw data;
      return { data: { data } };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({});
  api.post.mockResolvedValue({ data: { queued: true } });
  view = await mount(<ProvidersTab {...props} />);
}
const box = (name) => document.body.querySelector(`input[aria-label^="${name}:"]`);
const confirmDialog = () => document.body.querySelector('[role="dialog"]');
beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('ProvidersTab: which external services may be asked (#68)', () => {
  it('says for each provider whether it is on, what it receives and where it lives', async () => {
    await open();
    const text = view.text();
    expect(text).toContain('Todos começam desligados');
    expect(text).toContain('Open Library');
    expect(text).toContain('Recebe: o título da obra, o ISBN do arquivo, quando ele tem (para achar o livro sem dúvida), a chave de cada autor que você aceita (para obter os identificadores dele: Wikidata, VIAF, ISNI). Endereço: openlibrary.org (Internet Archive).');
    expect(text).toContain('Endereço: googleapis.com (Google).');
    expect([...document.body.querySelectorAll('li span.font-mono')].map((e) => e.textContent)).toEqual(['desligado', 'ligado', 'desligado']);
    expect(box('Open Library').checked).toBe(true);
    expect(box('Google Books').checked).toBe(false);
    expect(box('ComicVine').checked).toBe(false);
  });

  it('says what the manga providers are for, where they live and that they take no key, and turns one on only after the question', async () => {
    await open({ isOwner: true }, [...providers, provider('anilist', 'AniList', false), provider('mangadex', 'MangaDex', false)]);
    const text = view.text();
    expect(text).toContain('Recebe: o título da obra. Endereço: graphql.anilist.co (AniList).');
    expect(text).toContain('Endereço: api.mangadex.org (MangaDex).');
    expect(text).toContain('Para mangá: autores, gêneros, ano, sinopse e capa da série. Uso gratuito não comercial');
    expect(text).toContain('público (seinen, shounen…)');
    expect(text).not.toContain('chave de API: sem ela o AniList');
    await view.click(box('AniList'));
    expect(api.put).not.toHaveBeenCalled();
    expect(confirmDialog().textContent).toContain('o título da obra');
  });

  it('says Wikidata translates and identifies the work, that Wikipedia needs it, and that only the title of a page goes to Wikipedia', async () => {
    const wikipedia = { ...provider('wikipedia', 'Wikipedia', false), sends: ['page_title'] };
    await open({ isOwner: true }, [...providers, provider('wikidata', 'Wikidata', false, { sends: ['title', 'author_id'] }), wikipedia]);
    const text = view.text();
    expect(text).toContain('Recebe: o título da obra, o identificador Wikidata de cada autor que você aceita (para ler o perfil dele: descrição, anos, biografia e foto). Endereço: www.wikidata.org e commons.wikimedia.org (Wikimedia).');
    expect(text).toContain('Também lê o perfil dos autores que têm identificador Wikidata');
    expect(text).toContain('e a biografia dos autores');
    expect(text).toContain('traduz um título que os outros não conhecem');
    expect(text).toContain('Recebe: só o título da página da obra na Wikipédia, que o Wikidata informou (nunca o título do arquivo). Endereço: *.wikipedia.org (Wikimedia).');
    expect(text).toContain('Só funciona com o Wikidata ligado');
    expect(text).toContain('CC BY-SA 4.0');
    await view.click(box('Wikipedia'));
    expect(api.put).not.toHaveBeenCalled();
    expect(confirmDialog().textContent).toContain('é enviado a *.wikipedia.org (Wikimedia): só o título da página');
  });

  describe('how each answered the last time (DEC-144)', () => {
    const healthy = (over) => ({ state: 'ok', status: 200, problem: '', checkedAt: new Date().toISOString(), lastOkAt: new Date().toISOString(), empty: false, ...over });
    const noteOf = (state) => document.body.querySelector(`[data-health="${state}"]`);

    it('says, next to each provider, whether it is answering, and why it is not', async () => {
      await open({ isOwner: true }, [
        provider('google_books', 'Google Books', true, { key: 'required', keyConfigured: true, sends: ['title', 'isbn'], health: healthy({ state: 'key', status: 403 }) }),
        provider('openlibrary', 'Open Library', true, { sends: ['title', 'isbn', 'author_key'], health: healthy() }),
        provider('comicvine', 'ComicVine', true, { key: 'required', keyConfigured: true, health: healthy({ state: 'quota', status: 429 }) }),
        provider('anilist', 'AniList', true, { health: healthy({ state: 'down', status: 0, lastOkAt: null }) }),
        provider('mangadex', 'MangaDex', true),
        provider('wikidata', 'Wikidata', false),
      ]);
      expect(noteOf('key').textContent).toContain('A chave foi recusada (HTTP 403');
      expect(noteOf('key').textContent).toContain('Confira GOOGLE_BOOKS_API_KEY no ambiente do worker');
      expect(noteOf('key').className).toContain('text-danger');
      expect(noteOf('ok').textContent).toContain('Respondendo normalmente');
      expect(noteOf('ok').className).toContain('text-success');
      expect(noteOf('quota').textContent).toContain('O limite de uso foi atingido (HTTP 429');
      expect(noteOf('quota').className).toContain('text-warning');
      expect(noteOf('down').textContent).toContain('Não respondeu: rede ou serviço fora do ar');
      expect(noteOf('down').textContent).toContain('Ainda não respondeu bem.');
      expect(noteOf('none').textContent).toBe('Ainda não foi perguntado: nada a dizer sobre ele.');
      // a provider that is off and was never asked has nothing to say
      expect(document.body.querySelectorAll('[data-health]')).toHaveLength(5);
    });
  });

  describe('testing a provider (DEC-145)', () => {
    const testButton = (name) => document.body.querySelector(`button[aria-label="Testar ${name}"]`);
    const test = (state, over = {}) => ({ ok: state === 'ok', state, status: state === 'ok' ? 200 : 0, results: 0, ms: 840, testedAt: new Date().toISOString(), ...over });
    const noteOf = (state) => document.body.querySelector(`[data-test="${state}"]`);

    it('lets the owner test each provider, and nobody else', async () => {
      await open({ isOwner: true });
      expect(testButton('Google Books')).not.toBeNull();
      expect(testButton('Open Library').textContent).toBe('Testar');
      view.unmount();
      await open({ isOwner: false });
      expect(testButton('Open Library')).toBeNull();
    });

    it('asks at once for a provider that is on: it is already sent what it is asked', async () => {
      await open();
      await view.click(testButton('Open Library'));
      expect(api.post).toHaveBeenCalledWith('/admin/metadata-providers/openlibrary/test');
      expect(confirmDialog()).toBeNull();
    });

    it('says what the test asks before testing one that is off, and does nothing if the owner backs out', async () => {
      await open();
      await view.click(testButton('Google Books'));
      expect(api.post).not.toHaveBeenCalled();
      const text = confirmDialog().textContent;
      expect(text).toContain('Testar Google Books?');
      expect(text).toContain('googleapis.com (Google)');
      expect(text).toContain('um título fixo e público, “Dune”');
      expect(text).toContain('Nada da sua biblioteca é enviado. O provedor continua desligado.');
      expect(confirmDialog().textContent).not.toContain('chave de API configurada no worker vai junto');
      const cancel = [...confirmDialog().querySelectorAll('button')].find((b) => /cancelar/i.test(b.textContent));
      await view.click(cancel);
      expect(api.post).not.toHaveBeenCalled();
      expect(confirmDialog()).toBeNull();
      await view.click(testButton('Google Books'));
      await view.click([...confirmDialog().querySelectorAll('button')].find((b) => b.textContent === 'Testar'));
      expect(api.post).toHaveBeenCalledWith('/admin/metadata-providers/google_books/test');
      expect(api.put).not.toHaveBeenCalled(); // testing does not turn it on
    });

    it('says a test was asked for while the server has not answered yet, and only for that provider', async () => {
      await open();
      api.post.mockReturnValue(new Promise(() => {}));
      await view.click(testButton('Open Library'));
      expect(testButton('Open Library').disabled).toBe(true);
      expect(testButton('Open Library').textContent).toBe('Testando…');
      expect(testButton('Google Books').textContent).toBe('Testar');
      expect(testButton('ComicVine').textContent).toBe('Testar');
    });

    it('follows a test to its end: it asks again every few seconds while it runs, and stops when it is over', async () => {
      vi.useFakeTimers({ toFake: ['setTimeout', 'setInterval', 'clearTimeout', 'clearInterval'], shouldAdvanceTime: true });
      try {
        const rows = [provider('openlibrary', 'Open Library', true, { testing: true })];
        await open({ isOwner: true }, rows);
        const asked = () => api.get.mock.calls.filter(([url]) => url === '/admin/metadata-providers').length;
        const before = asked();
        await act(async () => { await vi.advanceTimersByTimeAsync(POLL_MS + 100); });
        expect(asked()).toBeGreaterThan(before);
        rows[0] = { ...rows[0], testing: false, test: { ok: true, state: 'ok', status: 200, results: 3, ms: 500, testedAt: new Date().toISOString() } };
        await act(async () => { await vi.advanceTimersByTimeAsync(POLL_MS + 100); });
        const settled = asked();
        await act(async () => { await vi.advanceTimersByTimeAsync(POLL_MS * 3); });
        expect(asked()).toBe(settled);
        expect(document.body.querySelector('[data-test="ok"]')).not.toBeNull();
      } finally {
        vi.useRealTimers();
      }
    });

    it('says the key goes along when the one that is off has a key', async () => {
      await open({ isOwner: true }, [provider('comicvine', 'ComicVine', false, { key: 'required', keyConfigured: true })]);
      await view.click(testButton('ComicVine'));
      expect(confirmDialog().textContent).toContain('“Absolute Batman”');
      expect(confirmDialog().textContent).toContain('A chave de API configurada no worker vai junto.');
    });

    it('says what is being asked of each: the page of Wikipedia, the title of a manga', async () => {
      await open({ isOwner: true }, [provider('wikipedia', 'Wikipedia', false, { sends: ['page_title'] }), provider('anilist', 'AniList', false)]);
      await view.click(testButton('Wikipedia'));
      expect(confirmDialog().textContent).toContain('a página “Dune (novel)”');
    });

    it('says a test is under way, and does not let it be asked twice', async () => {
      await open({ isOwner: true }, [provider('openlibrary', 'Open Library', true, { testing: true, test: test('ok', { results: 5 }) })]);
      expect(testButton('Open Library').disabled).toBe(true);
      expect(testButton('Open Library').textContent).toBe('Testando…');
      expect(view.text()).toContain('Testando…');
      expect(noteOf('ok')).toBeNull(); // the last one is not told while a new one runs
    });

    it('says how the last test came out, for each way it can', async () => {
      await open({ isOwner: true }, [
        provider('openlibrary', 'Open Library', true, { test: test('ok', { results: 12 }) }),
        provider('google_books', 'Google Books', true, { key: 'required', keyConfigured: true, test: test('key', { status: 400 }) }),
        provider('comicvine', 'ComicVine', true, { key: 'required', keyConfigured: true, test: test('nokey', { ms: 0 }) }),
        provider('anilist', 'AniList', true, { test: test('quota', { status: 429 }) }),
        provider('mangadex', 'MangaDex', true, { test: test('down') }),
        provider('wikidata', 'Wikidata', true, { test: test('empty', { status: 200 }) }),
        provider('wikipedia', 'Wikipedia', true, { test: test('error', { status: 404 }) }),
      ]);
      expect(noteOf('ok').textContent).toBe('Teste agora há pouco: respondeu em 0,8 s, com 12 resultados.');
      expect(noteOf('key').textContent).toContain('a chave foi recusada (HTTP 400). Confira GOOGLE_BOOKS_API_KEY');
      expect(noteOf('key').className).toContain('text-danger');
      expect(noteOf('nokey').textContent).toContain('faltou a chave de API, então nada foi perguntado. Defina COMICVINE_API_KEY');
      expect(noteOf('quota').textContent).toContain('o limite de uso foi atingido (HTTP 429)');
      expect(noteOf('down').textContent).toContain('não respondeu (rede ou serviço fora do ar)');
      expect(noteOf('empty').textContent).toContain('mas sem nada para uma pergunta que tem resposta');
      expect(noteOf('error').textContent).toContain('respondeu com erro (HTTP 404)');
    });

    it('says an error of the request', async () => {
      await open();
      api.post.mockRejectedValue({ response: { status: 404, data: 'Provedor não encontrado.' } });
      await view.click(testButton('Open Library'));
      expect(view.text()).toContain('Provedor não encontrado.');
      api.post.mockRejectedValue({ response: { status: 500, data: 'Error queueing the test' } });
      await view.click(testButton('Open Library'));
      expect(view.text()).toContain('Algo deu errado.');
    });

    it('asks again every few seconds while some provider is being tested, and never otherwise', () => {
      const data = (rows) => ({ state: { data: { data: rows } } });
      expect(pollWhileTesting(data([{ testing: false }, { testing: true }]))).toBeGreaterThan(0);
      expect(pollWhileTesting(data([{ testing: false }]))).toBe(false);
      expect(pollWhileTesting(data([]))).toBe(false);
      expect(pollWhileTesting({ state: {} })).toBe(false);
    });
  });

  it('turns one on only after saying what is sent, and not at all if the owner backs out', async () => {
    await open();
    await view.click(box('Google Books'));
    expect(api.put).not.toHaveBeenCalled();
    const dialogText = confirmDialog().textContent;
    expect(dialogText).toContain('Ligar Google Books?');
    expect(dialogText).toContain('googleapis.com (Google)');
    expect(dialogText).toContain('é enviado a googleapis.com (Google): o título da obra, o ISBN do arquivo, quando ele tem (para achar o livro sem dúvida).');
    expect(dialogText).toContain('O nome do arquivo, o conteúdo do livro e as notas não são enviados');
    expect(dialogText).not.toContain('chave de cada autor');

    await view.click(view.button('Cancelar'));
    expect(api.put).not.toHaveBeenCalled();
    expect(confirmDialog()).toBeNull();

    await view.click(box('Google Books'));
    await view.click([...document.body.querySelectorAll('[role="dialog"] button')].find((b) => b.textContent === 'Ligar'));
    expect(api.put).toHaveBeenCalledWith('/admin/metadata-providers/google_books', { enabled: true });
  });

  it('says before turning on Open Library that it is also asked about the authors that are accepted', async () => {
    await open({ isOwner: true }, [provider('openlibrary', 'Open Library', false, { sends: ['title', 'isbn', 'author_key'] })]);
    await view.click(box('Open Library'));
    const dialogText = confirmDialog().textContent;
    expect(dialogText).toContain('é enviado a openlibrary.org (Internet Archive): o título da obra, o ISBN do arquivo, quando ele tem (para achar o livro sem dúvida), a chave de cada autor que você aceita');
    expect(dialogText).toContain('Wikidata, VIAF, ISNI');
    expect(dialogText).toContain('O nome do arquivo, o conteúdo do livro e as notas não são enviados');
  });

  it('turns one off at once, with nothing to confirm', async () => {
    await open();
    await view.click(box('Open Library'));
    expect(confirmDialog()).toBeNull();
    expect(api.put).toHaveBeenCalledWith('/admin/metadata-providers/openlibrary', { enabled: false });
  });

  it('shows the choice to an admin but lets only the owner change it', async () => {
    await open({ isOwner: false });
    expect(box('Open Library').checked).toBe(true);
    for (const name of ['Google Books', 'Open Library', 'ComicVine']) expect(box(name).disabled).toBe(true);
    expect(view.text()).toContain('Só o dono do acervo liga ou desliga os provedores.');
  });

  it('does not say that only the owner changes it to the owner', async () => {
    await open({ isOwner: true });
    expect(view.text()).not.toContain('Só o owner liga ou desliga');
    expect(box('Google Books').disabled).toBe(false);
  });

  it('shows what the server said when the change fails', async () => {
    await open();
    api.put.mockRejectedValue({ response: { status: 403, data: 'Owner only' } });
    await view.click(box('Open Library'));
    expect(view.text()).toContain('Owner only');
  });

  it('says so when it could not load, and when there is nothing to choose', async () => {
    await open({ isOwner: true }, new Error('offline'));
    expect(view.text()).toContain('Não foi possível carregar os provedores.');
    expect(view.text()).not.toContain('Nenhum provedor.');
    view.unmount();
    await open({ isOwner: true }, []);
    expect(view.text()).toContain('Nenhum provedor.');
  });

  it('names a provider it knows nothing about plainly, without an address', async () => {
    await open({ isOwner: true }, [provider('novo', 'Novo', false, { sends: ['title', 'author'] })]);
    expect(view.text()).toContain('Recebe: o título da obra, author. Endereço: —.');
  });

  describe('the API keys, which are set in the environment of the worker', () => {
    const withKeys = (google, comic) => [
      provider('google_books', 'Google Books', false, { key: 'required', keyConfigured: google }),
      provider('openlibrary', 'Open Library', false),
      provider('comicvine', 'ComicVine', false, { key: 'required', keyConfigured: comic }),
    ];

    it('says a key is configured, and that it goes along when the provider is turned on', async () => {
      await open({ isOwner: true }, withKeys(true, true));
      expect(view.text().split('Chave de API configurada.')).toHaveLength(3); // twice: Google Books and ComicVine
      await view.click(box('ComicVine'));
      expect(confirmDialog().textContent).toContain('A chave de API configurada no worker vai junto.');
    });

    it('does not let a provider that cannot work without a key be turned on, and says what to do', async () => {
      await open({ isOwner: true }, withKeys(false, false));
      expect(view.text()).toContain('Falta a chave de API: sem ela o ComicVine não funciona. Defina COMICVINE_API_KEY no ambiente do worker e reinicie-o.');
      expect(view.text()).toContain('Falta a chave de API: sem ela o Google Books não funciona. Defina GOOGLE_BOOKS_API_KEY no ambiente do worker e reinicie-o.');
      expect(box('ComicVine').disabled).toBe(true);
      expect(box('Google Books').disabled).toBe(true);
      expect(box('Open Library').disabled).toBe(false);
      const noteOf = (text) => [...document.body.querySelectorAll('li p')].find((p) => p.textContent.includes(text));
      expect(noteOf('Falta a chave de API').className).toContain('text-danger');
    });

    it('lets Google Books be turned on once its key is there, and says why it needs one', async () => {
      await open({ isOwner: true }, withKeys(true, false));
      expect(view.text()).toContain('Só funciona com uma chave de API sua');
      expect(box('Google Books').disabled).toBe(false);
      expect(box('ComicVine').disabled).toBe(true);
      const note = [...document.body.querySelectorAll('li p')].find((p) => p.textContent.includes('Chave de API configurada'));
      expect(note.className).not.toContain('text-danger');
    });

    it('still lets one that is on be turned off when its key is gone', async () => {
      await open({ isOwner: true }, [provider('comicvine', 'ComicVine', true, { key: 'required', keyConfigured: false })]);
      expect(box('ComicVine').disabled).toBe(false);
      await view.click(box('ComicVine'));
      expect(api.put).toHaveBeenCalledWith('/admin/metadata-providers/comicvine', { enabled: false });
    });

    it('says the worker has not said yet when the key is needed', async () => {
      await open({ isOwner: true }, withKeys(null, null));
      expect(view.text().split('O worker ainda não informou se a chave de API existe.')).toHaveLength(3); // twice: Google Books and ComicVine
      expect(box('ComicVine').disabled).toBe(false);
      expect(box('Google Books').disabled).toBe(false);
    });

    it('says nothing about keys for a provider that has none', async () => {
      await open({ isOwner: true }, [provider('openlibrary', 'Open Library', false, { key: '', keyConfigured: true })]);
      expect(view.text()).not.toMatch(/chave de api/i);
    });
  });
});
