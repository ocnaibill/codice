import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));
vi.mock('../../../lib/download', () => ({ downloadFile: vi.fn() }));

import { api } from '../../../lib/api';
import { downloadFile } from '../../../lib/download';
import { mount, flush } from '../../admin/testUtils';
import { MyData } from './MyData';

let view;
const HOUR = 3600 * 1000;
const iso = (ms) => new Date(Date.now() + ms).toISOString();
const ready = (extra = {}) => ({ id: 'e1', state: 'ready', requestedAt: iso(-HOUR), readyAt: iso(-HOUR), expiresAt: iso(5 * HOUR + 600000), bytes: 2 * 1024 * 1024, ...extra });
const refusal = (status, data) => Object.assign(new Error(String(status)), { response: { status, data } });

let current;
async function open(item = null, error) {
  current = item;
  api.get.mockImplementation(async (url) => {
    if (url !== '/auth/export') throw new Error(`unexpected GET ${url}`);
    if (error) throw error;
    return { data: { export: current } };
  });
  api.post.mockImplementation(async () => { current = { id: 'e2', state: 'pending', requestedAt: iso(0) }; return { data: { id: 'e2' } }; });
  api.delete.mockImplementation(async () => { current = null; return {}; });
  view = await mount(<MyData />);
}
const text = () => view.text();

beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('MyData', () => {
  it('says what is in the file and what is not', async () => {
    await open();
    expect(text()).toContain('anotações e destaques');
    expect(text()).toContain('favoritos');
    expect(text()).toContain('o registro das suas entradas');
    expect(text()).toContain('Não tem');
    expect(text()).toContain('os livros');
    expect(text()).toContain('a sua senha');
  });

  it('offers to prepare the file when there is none, and asks for it', async () => {
    await open();
    await view.click(view.button('Preparar o meu arquivo'));
    expect(api.post).toHaveBeenCalledWith('/auth/export');
    expect(text()).toContain('Preparando o seu arquivo');
    expect(view.button('Preparar o meu arquivo')).toBeUndefined();
    expect(view.button('Baixar o arquivo')).toBeUndefined();
  });

  it('says it is being made, and offers nothing else', async () => {
    await open({ id: 'e1', state: 'pending', requestedAt: iso(0) });
    expect(text()).toContain('Preparando o seu arquivo');
    expect(text()).toContain('Pode levar um minuto');
    expect(view.button('Preparar o meu arquivo')).toBeUndefined();
    expect(view.button('Preparar outro')).toBeUndefined();
  });

  it('follows the file until it is ready', async () => {
    await open({ id: 'e1', state: 'pending', requestedAt: iso(0) });
    expect(text()).toContain('Preparando o seu arquivo');
    current = ready();
    // The screen asks again every few seconds: wait for it.
    await new Promise((resolve) => setTimeout(resolve, 3300));
    await flush();
    expect(text()).toContain('Seu arquivo está pronto');
  }, 10000);

  it('says the file is ready, how big, how long it stays and that only the person takes it', async () => {
    await open(ready());
    expect(text()).toContain('Seu arquivo está pronto');
    expect(text()).toContain('2,0 MB');
    expect(text()).toContain('mais 5 horas');
    expect(text()).toContain('só você consegue baixá-lo');
    expect(view.button('Baixar o arquivo')).toBeTruthy();
    expect(view.button('Preparar outro')).toBeTruthy();
    expect(view.button('Apagar o arquivo agora')).toBeTruthy();
  });

  it('takes the file with the session, saved under its own name', async () => {
    downloadFile.mockResolvedValue();
    await open(ready());
    await view.click(view.button('Baixar o arquivo'));
    expect(downloadFile).toHaveBeenCalledWith('/auth/export/e1/download', 'codice-meus-dados.zip');
  });

  it('says the file expired when the download is refused as gone, and asks again', async () => {
    downloadFile.mockRejectedValue(refusal(410, 'The export is not ready'));
    await open(ready());
    const before = api.get.mock.calls.length;
    current = ready({ state: 'expired' });
    await view.click(view.button('Baixar o arquivo'));
    await flush();
    expect(text()).toContain('O arquivo já expirou. Prepare outro.');
    expect(api.get.mock.calls.length).toBeGreaterThan(before);
  });

  it('says the same when the file is not found at all (taken away, or never there)', async () => {
    downloadFile.mockRejectedValue(refusal(404, 'Export not found'));
    await open(ready());
    await view.click(view.button('Baixar o arquivo'));
    expect(text()).toContain('O arquivo já expirou. Prepare outro.');
  });

  it('says a download that failed for another reason, plainly', async () => {
    downloadFile.mockRejectedValue(new Error('network'));
    await open(ready());
    await view.click(view.button('Baixar o arquivo'));
    expect(text()).toContain('Não foi possível baixar o arquivo. Tente de novo.');
  });

  it('removes the file when asked', async () => {
    await open(ready());
    await view.click(view.button('Apagar o arquivo agora'));
    expect(api.delete).toHaveBeenCalledWith('/auth/export/e1');
    await flush();
    expect(text()).not.toContain('Seu arquivo está pronto');
    expect(view.button('Preparar o meu arquivo')).toBeTruthy();
  });

  it('says it could not remove', async () => {
    await open(ready());
    api.delete.mockRejectedValue(new Error('down'));
    await view.click(view.button('Apagar o arquivo agora'));
    expect(text()).toContain('Não foi possível apagar o arquivo.');
  });

  it('replaces a file that is ready when another is asked', async () => {
    await open(ready());
    await view.click(view.button('Preparar outro'));
    expect(api.post).toHaveBeenCalledWith('/auth/export');
    expect(text()).toContain('Preparando o seu arquivo');
  });

  it('tells an expired file and lets the person ask again', async () => {
    await open(ready({ state: 'expired' }));
    expect(text()).toContain('O arquivo anterior expirou');
    expect(view.button('Baixar o arquivo')).toBeUndefined();
    expect(view.button('Preparar o meu arquivo')).toBeTruthy();
  });

  it('tells a failure and lets the person try again', async () => {
    await open(ready({ state: 'failed' }));
    expect(text()).toContain('Não foi possível preparar o arquivo');
    expect(text()).toContain('avise quem cuida do servidor');
    expect(view.button('Preparar o meu arquivo')).toBeTruthy();
  });

  it('says the limit in Portuguese when the server refuses with 429', async () => {
    await open();
    api.post.mockRejectedValue(refusal(429, 'Too many exports: try again in a while'));
    await view.click(view.button('Preparar o meu arquivo'));
    expect(text()).toContain('Você já pediu o arquivo 3 vezes na última hora');
  });

  it('says one is already being made when the server answers 409, without calling it a failure', async () => {
    await open();
    // Another window asked first: the server already has one in the making, and the screen must come to show it.
    api.post.mockImplementation(async () => {
      current = { id: 'e1', state: 'pending', requestedAt: iso(0) };
      throw refusal(409, { error: 'An export is already being made', id: 'e1' });
    });
    await view.click(view.button('Preparar o meu arquivo'));
    expect(text()).toContain('Já estamos preparando um arquivo para você.');
    expect(text()).toContain('Preparando o seu arquivo');
    expect(document.body.querySelector('[role="alert"]')).toBeNull();
  });

  it('says a request that failed in a way it has no sentence for with its own words', async () => {
    await open();
    api.post.mockRejectedValue(refusal(500, 'boom'));
    await view.click(view.button('Preparar o meu arquivo'));
    expect(text()).toContain('Não foi possível pedir o arquivo.');
    expect(text()).not.toContain('boom');
  });

  it('says it could not load, and tries again', async () => {
    await open(null, new Error('down'));
    expect(text()).toContain('Não foi possível ver o estado do seu arquivo.');
    expect(view.button('Preparar o meu arquivo')).toBeUndefined();
    expect(view.button('Tentar de novo')).toBeTruthy();
  });
});
