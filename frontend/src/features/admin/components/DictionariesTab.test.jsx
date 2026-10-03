import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { DictionariesTab } from './DictionariesTab';

let view;
afterEach(() => view?.unmount());
beforeEach(() => vi.clearAllMocks());

const links = { license: 'CC BY-SA 4.0 e GFDL', licenseUrl: 'https://creativecommons.org/licenses/by-sa/4.0/', source: 'Wikcionário, extraído com o Wiktextract (kaikki.org)', sourceUrl: 'https://kaikki.org/dictionary/rawdata.html' };
const pt = (extra = {}) => ({
  id: 'wikt-pt', name: 'Wikcionário em português', description: 'Definições em português e tradução.', edition: 'pt', installable: true,
  downloadBytes: 37158613, storageBytes: 326000000, state: 'available', progress: 0, bytesDone: 0, bytesTotal: null, entries: 0, forms: 0, links: 0,
  languages: [{ code: 'pt', level: 'complete' }, { code: 'en', level: 'partial' }, { code: 'ja', level: 'weak' }], ...links, ...extra,
});
const fr = () => ({
  id: 'wikt-fr', name: 'Wikcionário em francês', description: 'Definições em francês.', edition: 'fr', installable: false, downloadBytes: 733854991,
  state: 'available', progress: 0, bytesDone: 0, entries: 0, forms: 0, links: 0, languages: [{ code: 'fr', level: 'complete' }], ...links,
});

async function open(packages, { isOwner = true } = {}) {
  api.get.mockResolvedValue({ data: { packages } });
  api.post.mockResolvedValue({ data: {} });
  api.delete.mockResolvedValue({});
  view = await mount(<DictionariesTab isOwner={isOwner} />);
}
const card = (name) => [...view.container.querySelectorAll('li[aria-label]')].find((li) => li.getAttribute('aria-label') === name);
const inCard = (name, label) => [...card(name).querySelectorAll('button')].find((b) => b.textContent.trim() === label);
const dialog = () => document.body.querySelector('[role="dialog"]');
const choose = async (label) => view.click([...dialog().querySelectorAll('button')].find((b) => b.textContent.trim() === label));

describe('DictionariesTab: what the owner is told before anything is downloaded', () => {
  it('says the files are not the project\'s, that they come from a third party and that the owner downloads them', async () => {
    await open([pt(), fr()]);
    const note = view.container.querySelector('[role="note"]');
    expect(note.textContent).toContain('Estes arquivos não são do Códice');
    expect(note.textContent).toContain('terceiros');
    expect(note.textContent).toContain('Wikcionário');
    expect(note.textContent).toContain('kaikki.org');
    expect(note.textContent).toContain('o seu servidor');
    expect(note.textContent).toContain('CC BY-SA e GFDL');
    expect(note.textContent).toContain('endereço fixo');
    expect(note.textContent).toContain('Nenhuma palavra que você consulta sai do seu servidor');
  });

  it('lists every package with its languages, how well it covers each, its size and its license', async () => {
    await open([pt(), fr()]);
    const first = card('Wikcionário em português');
    expect(first.textContent).toContain('Definições em português e tradução.');
    expect([...first.querySelectorAll('ul[aria-label="Idiomas que traz"] li')].map((li) => li.textContent)).toEqual([
      'Português · completo', 'Inglês · parcial', 'Japonês · só tradução',
    ]);
    expect(first.textContent).toContain('Download de 35,4 MB · ocupa cerca de 310,9 MB no banco');
    const [license, source] = first.querySelectorAll('a');
    expect(license.getAttribute('href')).toBe('https://creativecommons.org/licenses/by-sa/4.0/');
    expect(license.textContent).toBe('CC BY-SA 4.0 e GFDL');
    expect(source.getAttribute('href')).toBe('https://kaikki.org/dictionary/rawdata.html');
    expect(card('Wikcionário em francês').textContent).toContain('Download de 699,9 MB');
    expect(license.getAttribute('target')).toBe('_blank');
    expect(license.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('shows a package that cannot be installed yet as coming, with nothing to press', async () => {
    await open([pt(), fr()]);
    expect(card('Wikcionário em francês').textContent).toContain('Em breve');
    expect(card('Wikcionário em francês').querySelectorAll('button').length).toBe(0);
    expect(card('Wikcionário em português').textContent).not.toContain('Em breve');
  });

  it('says it could not load them', async () => {
    api.get.mockRejectedValue(new Error('x'));
    view = await mount(<DictionariesTab isOwner />);
    expect(view.text()).toContain('Não foi possível carregar os dicionários.');
  });

  it('shows a placeholder while they load', async () => {
    api.get.mockReturnValue(new Promise(() => {}));
    view = await mount(<DictionariesTab isOwner />);
    expect(view.text()).toContain('Carregando');
  });
});

describe('DictionariesTab: installing', () => {
  it('asks first, saying what will be downloaded and from where, and downloads nothing until the owner says so', async () => {
    await open([pt()]);
    await view.click(inCard('Wikcionário em português', 'Instalar'));
    expect(dialog().textContent).toContain('Instalar Wikcionário em português?');
    expect(dialog().textContent).toContain('35,4 MB');
    expect(dialog().textContent).toContain('kaikki.org');
    expect(dialog().textContent).toContain('terceiros');
    expect(dialog().textContent).toContain('310,9 MB');
    expect(api.post).not.toHaveBeenCalled();
    await choose('Baixar e instalar');
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(api.post).toHaveBeenCalledWith('/admin/dictionaries/wikt-pt/install');
    expect(dialog()).toBeNull();
  });

  it('does nothing when the question is cancelled', async () => {
    await open([pt()]);
    await view.click(inCard('Wikcionário em português', 'Instalar'));
    await view.click(view.button('Cancelar'));
    expect(dialog()).toBeNull();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('does not say what it takes in the database when that was not measured', async () => {
    await open([pt({ storageBytes: 0 })]);
    await view.click(inCard('Wikcionário em português', 'Instalar'));
    expect(dialog().textContent).not.toContain('ocupar');
    expect(dialog().textContent).not.toContain('passa a ocupar');
  });

  it('says why when the server refuses', async () => {
    await open([pt()]);
    api.post.mockRejectedValue({ response: { status: 409, data: 'Este dicionário já está sendo instalado.' } });
    await view.click(inCard('Wikcionário em português', 'Instalar'));
    await choose('Baixar e instalar');
    expect(view.text()).toContain('Este dicionário já está sendo instalado.');
  });

  it('shows the owner nothing to press while it is being installed but the way to cancel', async () => {
    await open([pt({ state: 'installing', stage: 'downloading', progress: 0.5, bytesDone: 18579306, bytesTotal: 37158613 })]);
    expect(inCard('Wikcionário em português', 'Instalar')).toBeUndefined();
    expect(inCard('Wikcionário em português', 'Atualizar')).toBeUndefined();
    expect(inCard('Wikcionário em português', 'Cancelar')).toBeDefined();
  });

  it('shows how far the download is, in a bar and in words', async () => {
    await open([pt({ state: 'installing', stage: 'downloading', progress: 0.5, bytesDone: 18579306, bytesTotal: 37158613 })]);
    const bar = view.container.querySelector('[role="progressbar"]');
    expect(bar.getAttribute('aria-valuenow')).toBe('50');
    expect(bar.getAttribute('aria-label')).toBe('Andamento da instalação de Wikcionário em português');
    expect(bar.firstElementChild.style.width).toBe('50%');
    expect(card('Wikcionário em português').textContent).toContain('Baixando: 50% (17,7 MB de 35,4 MB)');
  });

  it('shows how far the import is, with the entries read', async () => {
    await open([pt({ state: 'installing', stage: 'importing', progress: 0.25, entries: 118730 })]);
    expect(view.container.querySelector('[role="progressbar"]').getAttribute('aria-valuenow')).toBe('25');
    expect(card('Wikcionário em português').textContent).toContain('Importando: 25% (118.730 verbetes lidos)');
  });

  it('says it is waiting for the worker, and that it is trying again', async () => {
    await open([pt({ state: 'installing', stage: 'queued' })]);
    expect(view.text()).toContain('Na fila, esperando o worker');
    view.unmount();
    await open([pt({ state: 'installing', stage: 'retrying', error: 'OSError: caiu' })]);
    expect(view.text()).toContain('Tentando de novo: OSError: caiu');
  });

  it('asks again while a package is being installed, so that it moves on its own, and stops when it is done', async () => {
    api.get.mockResolvedValueOnce({ data: { packages: [pt({ state: 'installing', stage: 'importing', progress: 0.1 })] } })
      .mockResolvedValue({ data: { packages: [pt({ state: 'ready', stage: 'done', progress: 1, entries: 456365, installedAt: '2026-10-03T10:00:00Z' })] } });
    view = await mount(<DictionariesTab isOwner />);
    expect(api.get).toHaveBeenCalledTimes(1);
    await new Promise((r) => setTimeout(r, 2200));
    await flush();
    expect(api.get.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(view.text()).toContain('456.365 verbetes');
    const calls = api.get.mock.calls.length;
    await new Promise((r) => setTimeout(r, 2200));
    await flush();
    expect(api.get.mock.calls.length).toBe(calls);
  });

  it('does not ask again when nothing is being installed', async () => {
    await open([pt()]);
    await new Promise((r) => setTimeout(r, 2200));
    await flush();
    expect(api.get).toHaveBeenCalledTimes(1);
  });
});

describe('DictionariesTab: cancelling', () => {
  const installing = () => pt({ state: 'installing', stage: 'importing', progress: 0.3 });

  it('asks first, saying what is lost and what is not, and cancels when told to', async () => {
    await open([installing()]);
    await view.click(inCard('Wikcionário em português', 'Cancelar'));
    expect(dialog().textContent).toContain('Cancelar a instalação de Wikcionário em português?');
    expect(dialog().textContent).toContain('continua valendo');
    expect(api.post).not.toHaveBeenCalled();
    await choose('Cancelar a instalação');
    expect(api.post).toHaveBeenCalledWith('/admin/dictionaries/wikt-pt/cancel');
  });

  it('lets the owner go on installing', async () => {
    await open([installing()]);
    await view.click(inCard('Wikcionário em português', 'Cancelar'));
    await view.click(view.button('Continuar instalando'));
    expect(dialog()).toBeNull();
    expect(api.post).not.toHaveBeenCalled();
  });
});

describe('DictionariesTab: what is installed', () => {
  const ready = () => pt({ state: 'ready', stage: 'done', progress: 1, entries: 456365, forms: 556654, links: 109875, installedAt: '2026-10-03T10:00:00Z', sourceDate: 'Mon, 28 Sep 2026 15:20:37 GMT' });

  it('says when it was installed, how many entries it has and the date of the file, and shows no bar', async () => {
    await open([ready()]);
    const text = card('Wikcionário em português').textContent;
    expect(text).toContain('Instalado em 03/10/2026');
    expect(text).toContain('456.365 verbetes');
    expect(text).toContain('arquivo de 28/09/2026');
    expect(view.container.querySelector('[role="progressbar"]')).toBeNull();
    expect(inCard('Wikcionário em português', 'Instalar')).toBeUndefined();
  });

  it('updates it after asking, and says the old one stays until the new one is complete', async () => {
    await open([ready()]);
    await view.click(inCard('Wikcionário em português', 'Atualizar'));
    expect(dialog().textContent).toContain('Atualizar Wikcionário em português?');
    expect(dialog().textContent).toContain('continua valendo');
    expect(api.post).not.toHaveBeenCalled();
    await choose('Baixar de novo');
    expect(api.post).toHaveBeenCalledWith('/admin/dictionaries/wikt-pt/install');
  });

  it('removes it after asking, and says what that means', async () => {
    await open([ready()]);
    await view.click(inCard('Wikcionário em português', 'Remover'));
    expect(dialog().textContent).toContain('Remover Wikcionário em português?');
    expect(dialog().textContent).toContain('saem do banco');
    expect(api.delete).not.toHaveBeenCalled();
    expect([...dialog().querySelectorAll('button')].find((b) => b.textContent.trim() === 'Remover').className).toContain('bg-red-700'); // what cannot be undone is red
    await choose('Remover');
    expect(api.delete).toHaveBeenCalledWith('/admin/dictionaries/wikt-pt');
  });

  it('does not remove it when the question is cancelled', async () => {
    await open([ready()]);
    await view.click(inCard('Wikcionário em português', 'Remover'));
    await view.click(view.button('Cancelar'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(dialog()).toBeNull();
  });
});

describe('DictionariesTab: when it did not work', () => {
  it('says why, and offers to try again', async () => {
    await open([pt({ state: 'failed', stage: 'importing', error: 'ValueError: o arquivo está incompleto' })]);
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('A instalação falhou: ValueError: o arquivo está incompleto');
    expect(inCard('Wikcionário em português', 'Tentar de novo')).toBeDefined();
    await view.click(inCard('Wikcionário em português', 'Tentar de novo'));
    expect(dialog().textContent).toContain('Instalar Wikcionário em português?');
  });

  it('says it was cancelled, in words, when it was', async () => {
    await open([pt({ state: 'failed', stage: 'cancelled', error: 'Instalação cancelada.' })]);
    expect(view.container.querySelector('[role="alert"]').textContent).toBe('A instalação foi cancelada.');
  });

  it('says so when it does not know why', async () => {
    await open([pt({ state: 'failed', stage: 'importing', error: '' })]);
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('motivo desconhecido');
  });
});

describe('DictionariesTab: someone who is not the owner', () => {
  it('sees where everything is and cannot press anything, and is told why', async () => {
    await open([pt({ state: 'ready', stage: 'done', progress: 1, entries: 5, installedAt: '2026-10-03T10:00:00Z' }), pt({ id: 'wikt-xx', name: 'Outro', state: 'installing', stage: 'importing', progress: 0.5 }), fr()], { isOwner: false });
    expect(view.container.querySelectorAll('li[aria-label] button').length).toBe(0);
    expect(view.text()).toContain('Só o dono do acervo instala, atualiza e remove dicionários.');
    expect(view.text()).toContain('Instalado em');
    expect(view.container.querySelector('[role="progressbar"]')).not.toBeNull();
  });

  it('is not told that for the owner', async () => {
    await open([pt()]);
    expect(view.text()).not.toContain('Só o dono do acervo');
  });
});

describe('DictionariesTab: finding the language to install', () => {
  const ja = () => pt({ id: 'wikt-ja', name: 'Wikcionário em japonês', edition: 'ja', downloadBytes: 64066672, storageBytes: 0, languages: [{ code: 'ja', level: 'complete' }] });
  const de = () => pt({ id: 'wikt-de', name: 'Wikcionário em alemão', edition: 'de', downloadBytes: 308579949, storageBytes: 0, languages: [{ code: 'de', level: 'complete' }] });
  const names = () => [...view.container.querySelectorAll('li[aria-label]')].map((li) => li.getAttribute('aria-label'));
  const search = () => view.container.querySelector('input[aria-label="Buscar um idioma"]');

  it('says how many dictionaries there are, and finds one by the name of its language with no accent', async () => {
    await open([pt({ languages: [{ code: 'pt', level: 'complete' }] }), de(), ja()]);
    expect(view.text()).toContain('3 dicionários');
    await view.type(search(), 'JAPONES');
    expect(names()).toEqual(['Wikcionário em japonês']);
    expect(view.text()).toContain('1 de 3 dicionários');
    await view.type(search(), 'alem');
    expect(names()).toEqual(['Wikcionário em alemão']);
  });

  it('finds a package that covers the language, and by its code', async () => {
    await open([pt({ languages: [{ code: 'pt', level: 'complete' }, { code: 'zh', level: 'weak' }] }), de()]);
    await view.type(search(), 'chinês');
    expect(names()).toEqual(['Wikcionário em português']);
    await view.type(search(), 'de');
    expect(names()).toContain('Wikcionário em alemão');
  });

  it('shows them all again when the search is cleared, and says when there is none', async () => {
    await open([pt(), de()]);
    await view.type(search(), 'xyzzy');
    expect(names()).toEqual([]);
    expect(view.text()).toContain('Nenhum dicionário para “xyzzy”.');
    expect(view.text()).toContain('0 de 2 dicionários');
    await view.type(search(), '');
    expect(names()).toEqual(['Wikcionário em português', 'Wikcionário em alemão']);
    expect(view.text()).not.toContain('Nenhum dicionário para');
  });

  it('puts what is installed first, then what can be installed', async () => {
    await open([de(), ja(), pt({ state: 'ready', stage: 'done', progress: 1, entries: 5, installedAt: '2026-10-03T10:00:00Z' })]);
    expect(names()).toEqual(['Wikcionário em português', 'Wikcionário em alemão', 'Wikcionário em japonês']);
  });

  it('lets the owner install any of them, and asks first', async () => {
    await open([ja(), de()]);
    await view.click(inCard('Wikcionário em japonês', 'Instalar'));
    expect(dialog().textContent).toContain('Instalar Wikcionário em japonês?');
    expect(dialog().textContent).toContain('61,1 MB');
    await choose('Baixar e instalar');
    expect(api.post).toHaveBeenCalledWith('/admin/dictionaries/wikt-ja/install');
  });

  it('warns that a big one takes long, and says nothing of the kind of a small one', async () => {
    await open([ja(), de()]);
    await view.click(inCard('Wikcionário em alemão', 'Instalar'));
    expect(dialog().textContent).toContain('294,3 MB');
    expect(dialog().textContent).toContain('É um arquivo grande');
    await view.click(view.button('Cancelar'));
    await view.click(inCard('Wikcionário em japonês', 'Instalar'));
    expect(dialog().textContent).not.toContain('É um arquivo grande');
  });
});
