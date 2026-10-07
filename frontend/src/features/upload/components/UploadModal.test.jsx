import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react';

vi.mock('../../../lib/api', () => ({ api: { post: vi.fn() } }));
vi.mock('../../../lib/refreshLibrary', () => ({ refreshLibrary: vi.fn() }));

import { api } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useToasts } from '../../../components/ui/toast';
import { mount, flush } from '../../admin/testUtils';
import { UploadModal } from './UploadModal';

let view;
let stamp = 0;
let posts; // the requests that are going, each waiting for the test to answer it
const file = (name, size = 2048) => new File([new Uint8Array(size)], name, { type: 'application/octet-stream', lastModified: ++stamp });
const input = () => document.body.querySelector('input[type="file"]');
const dropArea = () => document.body.querySelector('label[data-dragging]');
const dialog = () => document.body.querySelector('[role="dialog"]');
const rows = () => [...document.body.querySelectorAll('[aria-label="Arquivos para enviar"] > li')];
const row = (name) => rows().find((r) => r.textContent.includes(name));
const status = (name) => row(name)?.getAttribute('data-status');
const alertText = () => document.body.querySelector('[role="alert"]')?.textContent;
const named = (label) => [...document.body.querySelectorAll('button')].find((b) => b.getAttribute('aria-label') === label);
const choose = async (...files) => {
  await act(async () => {
    Object.defineProperty(input(), 'files', { value: files, configurable: true });
    input().dispatchEvent(new Event('change', { bubbles: true }));
  });
};
const dropOn = async (...files) => {
  await act(async () => {
    const event = new Event('drop', { bubbles: true, cancelable: true });
    event.dataTransfer = { files };
    dropArea().dispatchEvent(event);
  });
};
const sendButton = () => [...document.body.querySelectorAll('button')].find((b) => /^(Enviar|Enviando)/.test(b.textContent.trim()));
const sendAll = async () => { await act(async () => { sendButton().click(); }); await flush(); };
const answer = async (n, how = 'ok', value) => {
  await act(async () => {
    if (how === 'ok') posts[n].resolve({ data: value ?? {} });
    else posts[n].reject(value);
  });
  await flush();
};
const progress = async (n, loaded, total = 100) => { await act(async () => { posts[n].options.onUploadProgress({ loaded, total }); }); };
const open = async () => {
  useGlobalStore.setState({ isUploadModalOpen: true });
  view = await mount(<UploadModal />);
};
const fails = (status, data) => ({ response: { status, data } });
beforeEach(() => {
  vi.clearAllMocks();
  posts = [];
  api.post.mockImplementation((url, body, options) => new Promise((resolve, reject) => { posts.push({ url, body, options, resolve, reject }); }));
  useToasts.getState().clear();
  useGlobalStore.setState({ isUploadModalOpen: false });
});
afterEach(() => view.unmount());

describe('UploadModal, opening and closing', () => {
  it('is nothing while it is closed', async () => {
    view = await mount(<UploadModal />);
    expect(dialog()).toBeNull();
  });

  it('is a dialog with a name, in Portuguese, that fades in and rises', async () => {
    await open();
    expect(dialog().getAttribute('aria-modal')).toBe('true');
    expect(dialog().getAttribute('aria-label')).toBe('Adicionar à biblioteca');
    expect(view.text()).toContain('Adicionar à biblioteca');
    expect(view.text()).not.toMatch(/Add Book|Upload|\bCancel\b|Click to select/);
    expect(dialog().className).toContain('animate-pop-in');
    expect(dialog().parentElement.className).toContain('animate-fade-in');
    expect(document.activeElement).toBe(dialog());
  });

  it('closes by Escape, by the backdrop, by the X and by Cancelar, and forgets the files', async () => {
    for (const how of [
      () => act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); }),
      () => act(async () => { dialog().parentElement.click(); }),
      () => act(async () => { named('Fechar').click(); }),
      () => act(async () => { view.button('Cancelar').click(); }),
    ]) {
      await open();
      await choose(file('Duna.epub'), file('x.exe'));
      expect(rows()).toHaveLength(2);
      await how();
      expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
      await act(async () => { useGlobalStore.setState({ isUploadModalOpen: true }); });
      expect(rows()).toHaveLength(0);
      expect(alertText()).toBeUndefined();
      expect(sendButton().disabled).toBe(true);
      view.unmount();
    }
  });

  it('does not close for a click inside the dialog or for another key', async () => {
    await open();
    await act(async () => { dialog().click(); });
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' })); });
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
  });
});

describe('UploadModal, choosing files', () => {
  it('takes several at once, and lists each with its name, format and size', async () => {
    await open();
    expect(input().multiple).toBe(true);
    await choose(file('Duna.EPUB', 1536), file('Fundação.pdf', 3 * 1024 * 1024));
    expect(rows().map((r) => r.textContent)).toEqual(['epubDuna.EPUB1,5 KB', 'pdfFundação.pdf3,0 MB'].map((t) => expect.stringContaining(t.slice(0, 8))));
    expect(row('Duna.EPUB').textContent).toContain('1,5 KB');
    expect(row('Fundação.pdf').textContent).toContain('3,0 MB');
    expect(row('Duna.EPUB').textContent.toLowerCase()).toContain('epub');
    expect(status('Duna.EPUB')).toBe('queued');
  });

  it('says how many it will send, and offers nothing until there is a file', async () => {
    await open();
    expect(sendButton().disabled).toBe(true);
    expect(sendButton().textContent).toBe('Enviar');
    await choose(file('a.epub'));
    expect(sendButton().textContent).toBe('Enviar');
    expect(sendButton().disabled).toBe(false);
    await choose(file('b.epub'), file('c.epub'));
    expect(sendButton().textContent).toBe('Enviar 3 arquivos');
  });

  it('offers the formats it takes, and the same to the picker', async () => {
    await open();
    expect(view.text()).toContain('PDF, EPUB, CBZ, CBR, TXT, MD, MOBI e áudio');
    expect(view.text()).toContain('Escolha até 5 arquivos');
    expect(input().getAttribute('accept')).toContain('.epub');
    expect(input().getAttribute('accept')).toContain('.m4b');
  });

  it('keeps five at most, the first ones, and says how many were left out', async () => {
    await open();
    await choose(file('1.epub'), file('2.epub'), file('3.epub'));
    await choose(file('4.epub'), file('5.epub'), file('6.epub'));
    expect(rows()).toHaveLength(5);
    expect(row('6.epub')).toBeUndefined();
    expect(alertText()).toBe('Cabem 5 arquivos por vez: 1 ficou de fora. Envie esse depois.');
    expect(document.body.querySelector('[role="alert"]').className).toContain('bg-warning-soft');
    await act(async () => { named('Tirar 1.epub').click(); });
    await choose(file('7.epub'), file('8.epub'), file('9.epub'));
    expect(rows()).toHaveLength(5);
    expect(alertText()).toBe('Cabem 5 arquivos por vez: 2 ficaram de fora. Envie esses depois.');
  });

  it('says the list is full, and takes no more until a file is taken off', async () => {
    await open();
    await choose(...[1, 2, 3, 4, 5].map((n) => file(`${n}.epub`)));
    expect(view.text()).toContain('A lista está cheia: 5 arquivos');
    expect(input().disabled).toBe(true);
    expect(dropArea().className).toContain('opacity-60');
    await dropOn(file('6.epub'));
    expect(rows()).toHaveLength(5);
    expect(alertText()).toBeUndefined(); // the area already says it is full
    await act(async () => { named('Tirar 3.epub').click(); });
    expect(view.text()).toContain('Escolha até 5 arquivos');
    expect(input().disabled).toBe(false);
  });

  it('leaves out a file that is already on the list, and says so', async () => {
    await open();
    const same = file('Duna.epub');
    await choose(same);
    await choose(same, file('Outro.epub'));
    expect(rows()).toHaveLength(2);
    expect(alertText()).toBe('1 arquivo já estava na lista.');
    await choose(same, same);
    expect(alertText()).toBe('2 arquivos já estavam na lista.');
    await choose(same, file('1.epub'), file('2.epub'), file('3.epub'), file('4.epub'));
    expect(alertText()).toContain('Cabem 5 arquivos por vez: 1 ficou de fora');
    expect(alertText()).toContain('1 arquivo já estava na lista.');
  });

  it('takes the same name again when it is another file', async () => {
    await open();
    await choose(file('Duna.epub', 100));
    await choose(file('Duna.epub', 200));
    expect(rows()).toHaveLength(2);
    expect(alertText()).toBeUndefined();
  });

  it('lists a type it does not take as refused, with the reason, and does not count it for sending', async () => {
    await open();
    await choose(file('programa.exe'));
    expect(status('programa.exe')).toBe('refused');
    expect(row('programa.exe').textContent).toContain('Esse tipo de arquivo não é aceito (.exe).');
    expect(row('programa.exe').textContent).toContain('exe');
    expect(row('programa.exe').querySelector('.text-danger')).not.toBeNull();
    expect(sendButton().disabled).toBe(true);
    await choose(file('semextensao'));
    expect(row('semextensao').textContent).toContain('Esse tipo de arquivo não é aceito.');
    await choose(file('Duna.epub'));
    expect(sendButton().textContent).toBe('Enviar');
    expect(sendButton().disabled).toBe(false);
  });

  it('takes a file off the list, and lets it be chosen again', async () => {
    await open();
    const duna = file('Duna.epub');
    await choose(duna, file('b.epub'));
    await act(async () => { named('Tirar Duna.epub').click(); });
    expect(rows()).toHaveLength(1);
    await choose(duna);
    expect(rows()).toHaveLength(2);
    expect(alertText()).toBeUndefined();
  });

  it('does nothing when the picker is closed with no file', async () => {
    await open();
    await choose();
    expect(rows()).toHaveLength(0);
    expect(alertText()).toBeUndefined();
  });

  it('forgets what it said of the last choice when something else is chosen', async () => {
    await open();
    await choose(...[1, 2, 3, 4, 5, 6].map((n) => file(`${n}.epub`)));
    expect(alertText()).toBeDefined();
    await act(async () => { named('Tirar 1.epub').click(); });
    await choose(file('7.epub'));
    expect(alertText()).toBeUndefined();
  });
});

describe('UploadModal, dropping files', () => {
  it('lights the area while a file is over it, and puts it out when it leaves', async () => {
    await open();
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
    await act(async () => { dropArea().dispatchEvent(new Event('dragover', { bubbles: true, cancelable: true })); });
    expect(dropArea().getAttribute('data-dragging')).toBe('true');
    expect(dropArea().className).toContain('border-brand');
    await act(async () => { dropArea().dispatchEvent(new Event('dragleave', { bubbles: true })); });
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
  });

  it('stays lit while the file passes over what is inside the area', async () => {
    await open();
    await act(async () => { dropArea().dispatchEvent(new Event('dragenter', { bubbles: true, cancelable: true })); });
    await act(async () => {
      const leave = new Event('dragleave', { bubbles: true });
      leave.relatedTarget = dropArea().querySelector('p');
      dropArea().dispatchEvent(leave);
    });
    expect(dropArea().getAttribute('data-dragging')).toBe('true');
  });

  it('takes the files dropped on it, and puts the light out, with the page not opening them', async () => {
    await open();
    await act(async () => { dropArea().dispatchEvent(new Event('dragover', { bubbles: true, cancelable: true })); });
    let event;
    await act(async () => {
      event = new Event('drop', { bubbles: true, cancelable: true });
      event.dataTransfer = { files: [file('Duna.epub'), file('Fundação.epub')] };
      dropArea().dispatchEvent(event);
    });
    expect(event.defaultPrevented).toBe(true);
    expect(rows()).toHaveLength(2);
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
  });

  it('holds a dropped file to the same list as a chosen one, and to the same cap', async () => {
    await open();
    await dropOn(file('virus.exe'));
    expect(status('virus.exe')).toBe('refused');
    await dropOn(...[1, 2, 3, 4, 5].map((n) => file(`${n}.epub`)));
    expect(rows()).toHaveLength(5);
    expect(alertText()).toContain('1 ficou de fora');
  });

  it('does not light the area when it is full or sending', async () => {
    await open();
    await choose(...[1, 2, 3, 4, 5].map((n) => file(`${n}.epub`)));
    await act(async () => { dropArea().dispatchEvent(new Event('dragover', { bubbles: true, cancelable: true })); });
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
    await act(async () => { dropArea().dispatchEvent(new Event('dragenter', { bubbles: true, cancelable: true })); });
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
  });
});

describe('UploadModal, sending one file', () => {
  it('sends it as a form, with no time limit, and when it is done closes, says so and refreshes the library', async () => {
    await open();
    await choose(file('Duna.epub'));
    await sendAll();
    expect(posts).toHaveLength(1);
    expect(posts[0].url).toBe('/upload');
    expect(posts[0].body.get('document').name).toBe('Duna.epub');
    expect(posts[0].options.timeout).toBe(0);
    await answer(0);
    expect(refreshLibrary).toHaveBeenCalled();
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
    const notice = useToasts.getState().items[0];
    expect(notice).toMatchObject({ tone: 'success', title: 'Arquivo enviado' });
    expect(notice.message).toContain('Duna.epub');
  });
});

describe('UploadModal, sending several, one after the other', () => {
  it('starts the next only when the one before is over, and says which one it is on', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'), file('c.epub'));
    await sendAll();
    expect(posts).toHaveLength(1);
    expect(status('a.epub')).toBe('uploading');
    expect(status('b.epub')).toBe('queued');
    expect(document.body.querySelector('[role="status"]').textContent).toContain('Enviando 1 de 3');
    await answer(0);
    expect(posts).toHaveLength(2);
    expect(status('a.epub')).toBe('done');
    expect(status('b.epub')).toBe('uploading');
    expect(status('c.epub')).toBe('queued');
    expect(document.body.querySelector('[role="status"]').textContent).toContain('Enviando 2 de 3');
    expect(row('a.epub').querySelector('.text-success')).not.toBeNull();
    expect(row('a.epub').textContent).toContain('Enviado');
    await answer(1);
    await answer(2);
    expect(posts.map((p) => p.body.get('document').name)).toEqual(['a.epub', 'b.epub', 'c.epub']);
  });

  it('shows the progress of the file that is going, and only of it', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'));
    await sendAll();
    await progress(0, 40);
    const bars = document.body.querySelectorAll('[role="progressbar"]');
    expect(bars).toHaveLength(1);
    expect(bars[0].getAttribute('aria-label')).toBe('Andamento do envio de a.epub');
    expect(bars[0].getAttribute('aria-valuenow')).toBe('40');
    expect(bars[0].firstElementChild.style.width).toBe('40%');
    expect(row('a.epub').textContent).toContain('Enviando… 40%');
    expect(row('b.epub').textContent).toContain('2,0 KB');
    expect(document.body.querySelector('[role="status"]').textContent).not.toContain('preparando');
    await progress(0, 100);
    expect(document.body.querySelector('[role="status"]').textContent).toContain('entregue; preparando a leitura…');
  });

  it('closes when all went, with a notice that counts them', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'), file('c.epub'));
    await sendAll();
    await answer(0);
    await answer(1);
    await answer(2);
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
    expect(useToasts.getState().items).toHaveLength(1);
    expect(useToasts.getState().items[0]).toMatchObject({ tone: 'success', title: '3 arquivos enviados' });
    expect(refreshLibrary).toHaveBeenCalledTimes(3);
  });

  it('holds everything while it sends: nothing closes it, nothing is added, nothing is taken off', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'));
    await sendAll();
    expect(sendButton().textContent).toBe('Enviando…');
    expect(sendButton().disabled).toBe(true);
    expect(view.button('Cancelar').disabled).toBe(true);
    expect(named('Fechar').disabled).toBe(true);
    expect(named('Tirar b.epub').disabled).toBe(true);
    expect(named('Tirar a.epub')).toBeUndefined(); // the one that is going has no way out
    expect(input().disabled).toBe(true);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    await act(async () => { dialog().parentElement.click(); });
    await act(async () => { named('Fechar').click(); });
    await dropOn(file('c.epub'));
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
    expect(rows()).toHaveLength(2);
    await act(async () => { named('Tirar b.epub').click(); });
    expect(rows()).toHaveLength(2);
    await answer(0);
    await answer(1);
  });

  it('forgets what it said of the last choice when the sending begins', async () => {
    await open();
    await choose(...[1, 2, 3, 4, 5, 6].map((n) => file(`${n}.epub`)));
    expect(alertText()).toContain('1 ficou de fora');
    await sendAll();
    expect(alertText()).toBeUndefined();
    for (let n = 0; n < 5; n += 1) await answer(n, n === 4 ? 'fail' : 'ok', fails(413, 'x'));
    await act(async () => { named('Tentar de novo 5.epub').click(); });
    await flush();
    expect(alertText()).toBeUndefined();
    await answer(5);
  });

  it('forgets what it said of a choice when one file is sent again', async () => {
    await open();
    await choose(file('a.epub'));
    await sendAll();
    await answer(0, 'fail', fails(413, 'x'));
    await choose(...[1, 2, 3, 4, 5].map((n) => file(`n${n}.epub`)));
    expect(alertText()).toContain('ficou de fora');
    await act(async () => { named('Tentar de novo a.epub').click(); });
    await flush();
    expect(alertText()).toBeUndefined();
    await answer(1);
  });

  it('does not call the list full when what is on it was sent, and takes a new lot in its place', async () => {
    await open();
    await choose(...[1, 2, 3, 4, 5].map((n) => file(`${n}.epub`)));
    expect(view.text()).toContain('A lista está cheia: 5 arquivos');
    await sendAll();
    for (let n = 0; n < 4; n += 1) await answer(n);
    await answer(4, 'fail', fails(413, 'x'));
    expect(rows().map((r) => r.getAttribute('data-status'))).toEqual(['done', 'done', 'done', 'done', 'error']);
    expect(view.text()).not.toContain('A lista está cheia');
    expect(view.text()).toContain('Escolha até 5 arquivos');
    expect(input().disabled).toBe(false);
    await choose(...[6, 7, 8, 9, 10].map((n) => file(`${n}.epub`)));
    expect(rows().map((r) => r.querySelector('p').textContent)).toEqual(['5.epub', '6.epub', '7.epub', '8.epub', '9.epub']);
    expect(alertText()).toBe('Cabem 5 arquivos por vez: 1 ficou de fora. Envie esse depois.');
    expect(rows().filter((r) => r.getAttribute('data-status') === 'done')).toHaveLength(0);
  });

  it('says nothing of what is dropped while it sends, and puts nothing on the list', async () => {
    await open();
    await choose(file('a.epub'));
    await sendAll();
    await dropOn(file('b.epub'), file('c.epub'));
    expect(rows()).toHaveLength(1);
    expect(alertText()).toBeUndefined();
    await answer(0);
  });

  it('cannot be started twice', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'));
    await sendAll();
    await act(async () => { sendButton().click(); });
    await flush();
    expect(posts).toHaveLength(1);
    await answer(0);
    await answer(1);
  });
});

describe('UploadModal, stopping the rest', () => {
  it('lets the file that is going finish and cancels those that have not started', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'), file('c.epub'));
    await sendAll();
    await act(async () => { view.button('Parar o resto').click(); });
    expect(status('a.epub')).toBe('uploading');
    expect(named('Tentar de novo b.epub')).toBeUndefined(); // not while the one that is going has not ended
    expect(status('b.epub')).toBe('cancelled');
    expect(status('c.epub')).toBe('cancelled');
    await answer(0);
    expect(posts).toHaveLength(1);
    expect(status('a.epub')).toBe('done');
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
    expect(row('b.epub').textContent).toContain('Cancelado');
    expect(view.button('Parar o resto')).toBeUndefined();
    expect(named('Tentar de novo b.epub')).toBeDefined();
    expect(useToasts.getState().items[0]).toMatchObject({ tone: 'warning', title: '1 arquivo enviado' });
    expect(useToasts.getState().items[0].message).toBe('2 cancelados.');
  });

  it('is offered only while it sends', async () => {
    await open();
    expect(view.button('Parar o resto')).toBeUndefined();
    await choose(file('a.epub'));
    expect(view.button('Parar o resto')).toBeUndefined();
    await sendAll();
    expect(view.button('Parar o resto')).toBeDefined();
    await answer(0);
  });

  it('does nothing for a stop when nothing is sending', async () => {
    await open();
    await choose(file('a.epub'));
    expect(status('a.epub')).toBe('queued');
  });

  it('lets a cancelled file be sent again, alone', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'));
    await sendAll();
    await act(async () => { view.button('Parar o resto').click(); });
    await answer(0);
    await act(async () => { named('Tentar de novo b.epub').click(); });
    await flush();
    expect(posts).toHaveLength(2);
    expect(posts[1].body.get('document').name).toBe('b.epub');
    await answer(1);
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
  });
});

describe('UploadModal, when one fails', () => {
  it('goes on with the others, says why for that one, and keeps the dialog open', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'), file('c.epub'));
    await sendAll();
    await answer(0);
    await answer(1, 'fail', fails(413, 'x'));
    expect(posts).toHaveLength(3);
    await answer(2);
    expect(status('b.epub')).toBe('error');
    expect(row('b.epub').textContent).toContain('O arquivo é grande demais.');
    expect(row('b.epub').querySelector('.text-danger')).not.toBeNull();
    expect(status('a.epub')).toBe('done');
    expect(status('c.epub')).toBe('done');
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
    expect(useToasts.getState().items[0]).toMatchObject({ tone: 'warning', title: '2 arquivos enviados' });
    expect(useToasts.getState().items[0].message).toBe('1 com erro.');
    expect(view.button('Fechar')).toBeDefined();
    expect(view.button('Cancelar')).toBeUndefined();
  });

  it('sends only that one again, and closes when everything is in', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'));
    await sendAll();
    await answer(0);
    await answer(1, 'fail', {});
    expect(row('b.epub').textContent).toContain('Não foi possível falar com o servidor.');
    await act(async () => { named('Tentar de novo b.epub').click(); });
    await flush();
    expect(posts).toHaveLength(3);
    expect(posts[2].body.get('document').name).toBe('b.epub');
    expect(status('b.epub')).toBe('uploading');
    expect(named('Tentar de novo b.epub')).toBeUndefined();
    await answer(2);
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
    expect(useToasts.getState().items.at(-1)).toMatchObject({ tone: 'success', title: 'Arquivo enviado' });
  });

  it('says a text was brought to UTF-8, by name and by the encoding it came in', async () => {
    await open();
    await choose(file('antigo.txt'));
    await sendAll();
    await answer(0, 'ok', { work_id: 1, converted_from: 'Windows-1252' });
    const notice = useToasts.getState().items[0];
    expect(notice).toMatchObject({ tone: 'success', title: 'Arquivo enviado' });
    expect(notice.message).toContain('“antigo.txt” foi convertido de Windows-1252 para UTF-8.');
  });

  it('names only the files that were converted, when several go, and keeps it in the warning', async () => {
    await open();
    await choose(file('a.txt'), file('b.txt'), file('c.epub'));
    await sendAll();
    await answer(0, 'ok', { converted_from: 'UTF-16 LE' });
    await answer(1);
    await answer(2, 'fail', fails(409, { title: 'Duna', retired: false }));
    const notice = useToasts.getState().items[0];
    expect(notice).toMatchObject({ tone: 'warning', title: '2 arquivos enviados' });
    expect(notice.message).toContain('“a.txt” foi convertido de UTF-16 LE para UTF-8.');
    expect(notice.message).not.toContain('b.txt');
  });

  it('says nothing about a conversion when there was none', async () => {
    await open();
    await choose(file('Duna.epub'));
    await sendAll();
    await answer(0, 'ok', { work_id: 1 });
    expect(useToasts.getState().items[0].message).not.toContain('UTF-8');
  });

  it('keeps the note on the row of a converted file that stays on the list', async () => {
    await open();
    await choose(file('antigo.txt'), file('virus.exe'));
    await sendAll();
    await answer(0, 'ok', { converted_from: 'Windows-1252' });
    expect(row('antigo.txt').textContent).toContain('Enviado · Convertido de Windows-1252 para UTF-8.');
  });

  it('says that a file is already in the library as a warning, with no way to send it again', async () => {
    await open();
    await choose(file('a.epub'), file('Duna.epub'));
    await sendAll();
    await answer(0);
    await answer(1, 'fail', fails(409, { title: 'Duna', retired: true }));
    expect(status('Duna.epub')).toBe('duplicate');
    expect(row('Duna.epub').textContent).toContain('Esse arquivo já está no acervo como “Duna” (na lixeira).');
    expect(row('Duna.epub').querySelector('.text-warning')).not.toBeNull();
    expect(named('Tentar de novo Duna.epub')).toBeUndefined();
    expect(useToasts.getState().items[0].message).toBe('1 já estava no acervo.');
  });

  it('says in Portuguese what the server said of the content', async () => {
    await open();
    await choose(file('Duna.epub'));
    await sendAll();
    await answer(0, 'fail', fails(415, 'the content is not a valid .epub file\n'));
    expect(row('Duna.epub').textContent).toContain('the content is not a valid .epub file');
    expect(useToasts.getState().items).toHaveLength(0); // nothing went
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
  });

  it('says nothing of what went when nothing did, and the dialog stays', async () => {
    await open();
    await choose(file('a.epub'), file('b.epub'));
    await sendAll();
    await answer(0, 'fail', fails(413, 'x'));
    await answer(1, 'fail', fails(403, 'x'));
    expect(useToasts.getState().items).toHaveLength(0);
    expect(row('b.epub').textContent).toContain('Só quem administra o acervo pode adicionar arquivos.');
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
  });

  it('counts the three kinds of trouble in one notice', async () => {
    await open();
    await choose(file('ok.epub'), file('dup.epub'), file('bad.epub'), file('later.epub'));
    await sendAll();
    await answer(0);
    await answer(1, 'fail', fails(409, { title: 'X' }));
    await answer(2, 'fail', fails(500, 'boom'));
    await act(async () => { view.button('Parar o resto').click(); });
    await answer(3);
    expect(useToasts.getState().items[0].title).toBe('2 arquivos enviados');
  });

  it('does not close for a file that was refused: it stays on the list to be seen', async () => {
    await open();
    await choose(file('Duna.epub'), file('virus.exe'));
    await sendAll();
    await answer(0);
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
    expect(status('virus.exe')).toBe('refused');
    expect(useToasts.getState().items[0]).toMatchObject({ tone: 'success', title: 'Arquivo enviado' });
    expect(view.button('Fechar')).toBeDefined();
  });

  it('forgets everything when the dialog is closed and opened again', async () => {
    await open();
    await choose(file('a.epub'));
    await sendAll();
    await answer(0, 'fail', fails(413, 'x'));
    await act(async () => { view.button('Fechar').click(); });
    await act(async () => { useGlobalStore.setState({ isUploadModalOpen: true }); });
    expect(rows()).toHaveLength(0);
    expect(view.button('Cancelar')).toBeDefined();
  });
});
