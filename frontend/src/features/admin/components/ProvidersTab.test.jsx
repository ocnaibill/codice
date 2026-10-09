import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { ProvidersTab } from './ProvidersTab';

const provider = (id, name, enabled, over = {}) => ({ id, name, enabled, sends: ['title'], key: '', keyConfigured: null, ...over });
const providers = [
  provider('google_books', 'Google Books', false, { key: 'optional' }),
  provider('openlibrary', 'Open Library', true, { sends: ['title', 'author_key'] }),
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
    expect(text).toContain('Recebe: o título da obra, a chave de cada autor que você aceita (para obter os identificadores dele: Wikidata, VIAF, ISNI). Endereço: openlibrary.org (Internet Archive).');
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
    await open({ isOwner: true }, [...providers, provider('wikidata', 'Wikidata', false), wikipedia]);
    const text = view.text();
    expect(text).toContain('Recebe: o título da obra. Endereço: www.wikidata.org (Wikimedia).');
    expect(text).toContain('traduz um título que os outros não conhecem');
    expect(text).toContain('Recebe: só o título da página da obra na Wikipédia, que o Wikidata informou (nunca o título do arquivo). Endereço: *.wikipedia.org (Wikimedia).');
    expect(text).toContain('Só funciona com o Wikidata ligado');
    expect(text).toContain('CC BY-SA 4.0');
    await view.click(box('Wikipedia'));
    expect(api.put).not.toHaveBeenCalled();
    expect(confirmDialog().textContent).toContain('é enviado a *.wikipedia.org (Wikimedia): só o título da página');
  });

  it('turns one on only after saying what is sent, and not at all if the owner backs out', async () => {
    await open();
    await view.click(box('Google Books'));
    expect(api.put).not.toHaveBeenCalled();
    const dialogText = confirmDialog().textContent;
    expect(dialogText).toContain('Ligar Google Books?');
    expect(dialogText).toContain('googleapis.com (Google)');
    expect(dialogText).toContain('é enviado a googleapis.com (Google): o título da obra.');
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
    await open({ isOwner: true }, [provider('openlibrary', 'Open Library', false, { sends: ['title', 'author_key'] })]);
    await view.click(box('Open Library'));
    const dialogText = confirmDialog().textContent;
    expect(dialogText).toContain('é enviado a openlibrary.org (Internet Archive): o título da obra, a chave de cada autor que você aceita');
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
      provider('google_books', 'Google Books', false, { key: 'optional', keyConfigured: google }),
      provider('openlibrary', 'Open Library', false),
      provider('comicvine', 'ComicVine', false, { key: 'required', keyConfigured: comic }),
    ];

    it('says a key is configured, and that it goes along when the provider is turned on', async () => {
      await open({ isOwner: true }, withKeys(true, true));
      expect(view.text()).toContain('Chave de API configurada: o limite de uso é maior.');
      expect(view.text()).toContain('Chave de API configurada.');
      await view.click(box('ComicVine'));
      expect(confirmDialog().textContent).toContain('A chave de API configurada no worker vai junto.');
    });

    it('says a key that can be done without is missing, and still lets the provider be turned on', async () => {
      await open({ isOwner: true }, withKeys(false, true));
      expect(view.text()).toContain('Sem chave de API: funciona, com limite de uso menor. Para aumentar, defina GOOGLE_BOOKS_API_KEY no ambiente do worker.');
      expect(box('Google Books').disabled).toBe(false);
      await view.click(box('Google Books'));
      expect(confirmDialog().textContent).not.toContain('chave de API configurada no worker vai junto');
    });

    it('does not let a provider that cannot work without a key be turned on, and says what to do', async () => {
      await open({ isOwner: true }, withKeys(true, false));
      expect(view.text()).toContain('Falta a chave de API: sem ela o ComicVine não funciona. Defina COMICVINE_API_KEY no ambiente do worker e reinicie-o.');
      expect(box('ComicVine').disabled).toBe(true);
      expect(box('Google Books').disabled).toBe(false);
      const noteOf = (text) => [...document.body.querySelectorAll('li p')].find((p) => p.textContent.includes(text));
      expect(noteOf('Falta a chave de API').className).toContain('text-danger');
      expect(noteOf('Chave de API configurada').className).not.toContain('text-danger');
    });

    it('still lets one that is on be turned off when its key is gone', async () => {
      await open({ isOwner: true }, [provider('comicvine', 'ComicVine', true, { key: 'required', keyConfigured: false })]);
      expect(box('ComicVine').disabled).toBe(false);
      await view.click(box('ComicVine'));
      expect(api.put).toHaveBeenCalledWith('/admin/metadata-providers/comicvine', { enabled: false });
    });

    it('says the worker has not said yet when it is a key that is needed, and nothing when it can be done without', async () => {
      await open({ isOwner: true }, withKeys(null, null));
      expect(view.text().split('O worker ainda não informou se a chave de API existe.')).toHaveLength(2); // once: ComicVine only
      expect(view.text()).not.toContain('Sem chave de API');
      expect(box('ComicVine').disabled).toBe(false);
    });

    it('says nothing about keys for a provider that has none', async () => {
      await open({ isOwner: true }, [provider('openlibrary', 'Open Library', false, { key: '', keyConfigured: true })]);
      expect(view.text()).not.toMatch(/chave de api/i);
    });
  });
});
