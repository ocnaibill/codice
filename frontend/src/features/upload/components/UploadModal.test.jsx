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
const file = (name, size = 2048) => new File([new Uint8Array(size)], name, { type: 'application/octet-stream' });
const input = () => document.body.querySelector('input[type="file"]');
const dropArea = () => document.body.querySelector('label[data-dragging]');
const dialog = () => document.body.querySelector('[role="dialog"]');
const alertText = () => document.body.querySelector('[role="alert"]')?.textContent;
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
const send = async () => { await act(async () => { view.button('Enviar').click(); }); await flush(); };
const open = async () => {
  useGlobalStore.setState({ isUploadModalOpen: true });
  view = await mount(<UploadModal />);
};
beforeEach(() => {
  vi.clearAllMocks();
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

  it('closes by Escape, by the backdrop, by the X and by Cancelar, and forgets the file', async () => {
    for (const how of [
      () => act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); }),
      () => act(async () => { dialog().parentElement.click(); }),
      () => act(async () => { document.querySelector('[aria-label="Fechar"]').click(); }),
      () => act(async () => { view.button('Cancelar').click(); }),
    ]) {
      await open();
      await choose(file('Duna.epub'));
      expect(view.text()).toContain('Duna.epub');
      await how();
      expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
      // The same dialog opens again, and what was chosen is not there.
      await act(async () => { useGlobalStore.setState({ isUploadModalOpen: true }); });
      expect(view.text()).not.toContain('Duna.epub');
      expect(view.button('Enviar').disabled).toBe(true);
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

describe('UploadModal, choosing a file', () => {
  it('shows the name, the format and the size, and only then lets it be sent', async () => {
    await open();
    expect(view.button('Enviar').disabled).toBe(true);
    await choose(file('Duna.EPUB', 1536));
    expect(view.text()).toContain('Duna.EPUB');
    expect(view.text()).toContain('1,5 KB');
    expect(document.body.textContent).toContain('epub');
    expect(view.button('Enviar').disabled).toBe(false);
  });

  it('offers the formats it takes, and the same to the picker', async () => {
    await open();
    expect(view.text()).toContain('PDF, EPUB, CBZ, CBR, TXT, MD, MOBI e áudio');
    expect(input().getAttribute('accept')).toContain('.epub');
    expect(input().getAttribute('accept')).toContain('.m4b');
  });

  it('refuses a type it does not take, and says which, without keeping it', async () => {
    await open();
    await choose(file('programa.exe'));
    expect(alertText()).toContain('Esse tipo de arquivo não é aceito (.exe)');
    expect(document.body.querySelector('[role="alert"]').className).toContain('bg-danger-soft');
    expect(view.button('Enviar').disabled).toBe(true);
    expect(view.text()).not.toContain('programa.exe');
    await choose(file('semextensao'));
    expect(alertText()).toContain('Esse tipo de arquivo não é aceito.');
  });

  it('takes the new file in the place of the one before, and clears what was said', async () => {
    await open();
    await choose(file('a.exe'));
    expect(alertText()).toBeDefined();
    await choose(file('b.epub'));
    expect(alertText()).toBeUndefined();
    expect(view.text()).toContain('b.epub');
    await choose(file('c.pdf'));
    expect(view.text()).toContain('c.pdf');
    expect(view.text()).not.toContain('b.epub');
  });

  it('takes the file off again', async () => {
    await open();
    await choose(file('Duna.epub'));
    await act(async () => { document.querySelector('[aria-label="Tirar Duna.epub"]').click(); });
    expect(view.text()).not.toContain('Duna.epub');
    expect(view.button('Enviar').disabled).toBe(true);
  });

  it('lets the same file be chosen again after it was taken off', async () => {
    await open();
    await choose(file('Duna.epub'));
    expect(input().value).toBe('');
  });

  it('does nothing when the picker is closed with no file', async () => {
    await open();
    await choose();
    expect(view.button('Enviar').disabled).toBe(true);
    expect(alertText()).toBeUndefined();
  });
});

describe('UploadModal, dropping a file', () => {
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

  it('takes a file dropped on it, and puts the light out', async () => {
    await open();
    await act(async () => { dropArea().dispatchEvent(new Event('dragover', { bubbles: true, cancelable: true })); });
    let event;
    await act(async () => {
      event = new Event('drop', { bubbles: true, cancelable: true });
      event.dataTransfer = { files: [file('Duna.epub')] };
      dropArea().dispatchEvent(event);
    });
    expect(event.defaultPrevented).toBe(true);
    expect(view.text()).toContain('Duna.epub');
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
  });

  it('holds a dropped file to the same list as a chosen one', async () => {
    await open();
    await dropOn(file('virus.exe'));
    expect(alertText()).toContain('(.exe)');
    expect(view.button('Enviar').disabled).toBe(true);
  });

  it('takes the first of several and says that one goes at a time', async () => {
    await open();
    await dropOn(file('um.epub'), file('dois.epub'));
    expect(view.text()).toContain('um.epub');
    expect(view.text()).not.toContain('dois.epub');
    expect(alertText()).toContain('Um arquivo por vez');
  });
});

describe('UploadModal, sending', () => {
  it('sends the file as a form, with no time limit, and when it is done closes, says so and refreshes the library', async () => {
    api.post.mockResolvedValue({ data: { work_id: 1 } });
    await open();
    await choose(file('Duna.epub'));
    await send();
    expect(api.post).toHaveBeenCalledTimes(1);
    const [url, body, options] = api.post.mock.calls[0];
    expect(url).toBe('/upload');
    expect(body.get('document').name).toBe('Duna.epub');
    expect(options.timeout).toBe(0);
    expect(refreshLibrary).toHaveBeenCalled();
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
    const notice = useToasts.getState().items[0];
    expect(notice).toMatchObject({ tone: 'success', title: 'Arquivo enviado' });
    expect(notice.message).toContain('Duna.epub');
  });

  it('shows how far it is, and that it is preparing the reading when it is all delivered', async () => {
    let finish;
    api.post.mockImplementation((url, body, options) => new Promise((resolve) => {
      options.onUploadProgress({ loaded: 25, total: 100 });
      finish = (data) => resolve({ data });
    }));
    await open();
    await choose(file('Duna.epub'));
    await send();
    const bar = () => document.body.querySelector('[role="progressbar"]');
    expect(bar().getAttribute('aria-valuenow')).toBe('25');
    expect(bar().firstElementChild.style.width).toBe('25%');
    expect(view.text()).toContain('Enviando…');
    expect(view.text()).toContain('25%');
    await act(async () => { api.post.mock.calls[0][2].onUploadProgress({ loaded: 100, total: 100 }); });
    expect(view.text()).toContain('Entregue; preparando a leitura…');
    expect(bar().getAttribute('aria-valuenow')).toBe('100');
    await act(async () => { finish({}); });
    await flush();
  });

  it('ignores a progress report with no total', async () => {
    let finish;
    api.post.mockImplementation((url, body, options) => new Promise((resolve) => {
      options.onUploadProgress({ loaded: 5 });
      finish = () => resolve({ data: {} });
    }));
    await open();
    await choose(file('Duna.epub'));
    await send();
    expect(document.body.querySelector('[role="progressbar"]').getAttribute('aria-valuenow')).toBe('0');
    await act(async () => { finish(); });
    await flush();
  });

  it('holds everything while it sends: nothing closes it, nothing changes the file, and it cannot be sent twice', async () => {
    let finish;
    api.post.mockImplementation(() => new Promise((resolve) => { finish = () => resolve({ data: {} }); }));
    await open();
    await choose(file('Duna.epub'));
    await send();
    expect(view.button('Enviando…').disabled).toBe(true);
    expect(view.button('Cancelar').disabled).toBe(true);
    expect(document.querySelector('[aria-label="Fechar"]').disabled).toBe(true);
    expect(document.querySelector('[aria-label="Tirar Duna.epub"]').disabled).toBe(true);
    expect(input().disabled).toBe(true);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    await act(async () => { dialog().parentElement.click(); });
    await act(async () => { document.querySelector('[aria-label="Fechar"]').click(); });
    await dropOn(file('Outro.epub'));
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
    expect(view.text()).toContain('Duna.epub');
    expect(view.text()).not.toContain('Outro.epub');
    expect(api.post).toHaveBeenCalledTimes(1);
    await act(async () => { finish(); });
    await flush();
  });

  it('does not light the area for a file dragged over while it sends', async () => {
    api.post.mockImplementation(() => new Promise(() => {}));
    await open();
    await choose(file('Duna.epub'));
    await send();
    await act(async () => { dropArea().dispatchEvent(new Event('dragover', { bubbles: true, cancelable: true })); });
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
    await act(async () => { dropArea().dispatchEvent(new Event('dragenter', { bubbles: true, cancelable: true })); });
    expect(dropArea().getAttribute('data-dragging')).toBe('false');
  });

  it('keeps saying it is sending until it is all delivered', async () => {
    api.post.mockImplementation((url, body, options) => new Promise(() => { options.onUploadProgress({ loaded: 99, total: 100 }); }));
    await open();
    await choose(file('Duna.epub'));
    await send();
    expect(view.text()).toContain('Enviando…');
    expect(view.text()).toContain('99%');
    expect(view.text()).not.toContain('preparando a leitura');
  });
});

describe('UploadModal, when it fails', () => {
  const fail = (status, data) => { api.post.mockRejectedValue({ response: { status, data } }); };

  it('says in Portuguese that the file is already there, keeps the dialog open and lets another be chosen', async () => {
    fail(409, { title: 'Duna', retired: false });
    await open();
    await choose(file('Duna.epub'));
    await send();
    expect(alertText()).toBe('Esse arquivo já está no acervo como “Duna”.');
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
    expect(view.button('Enviar').disabled).toBe(false);
    expect(useToasts.getState().items).toHaveLength(0);
    await choose(file('Outro.epub'));
    expect(alertText()).toBeUndefined();
  });

  it('relays what the server says of the content, and of the size, in a box of danger', async () => {
    fail(415, 'the content is not a valid .epub file\n');
    await open();
    await choose(file('Duna.epub'));
    await send();
    expect(alertText()).toBe('the content is not a valid .epub file');
    expect(document.body.querySelector('[role="alert"]').className).toContain('bg-danger-soft');
    fail(413, 'x');
    await send();
    expect(alertText()).toBe('O arquivo é grande demais.');
  });

  it('says so when the server cannot be reached, and a retry can work', async () => {
    api.post.mockRejectedValueOnce({});
    api.post.mockResolvedValueOnce({ data: {} });
    await open();
    await choose(file('Duna.epub'));
    await send();
    expect(alertText()).toBe('Não foi possível falar com o servidor.');
    await send();
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
  });

  it('shows the advice about one file at a time as a warning, not as an error', async () => {
    await open();
    await dropOn(file('um.epub'), file('dois.epub'));
    expect(document.body.querySelector('[role="alert"]').className).toContain('bg-warning-soft');
  });

  it('forgets the error when the dialog is closed and opened again', async () => {
    fail(413, 'x');
    await open();
    await choose(file('Duna.epub'));
    await send();
    await act(async () => { view.button('Cancelar').click(); });
    await act(async () => { useGlobalStore.setState({ isUploadModalOpen: true }); });
    expect(alertText()).toBeUndefined();
    expect(view.text()).not.toContain('Duna.epub');
  });

  it('forgets what it said of a type it does not take when the dialog is closed and opened again', async () => {
    await open();
    await choose(file('a.exe'));
    expect(alertText()).toBeDefined();
    await act(async () => { view.button('Cancelar').click(); });
    await act(async () => { useGlobalStore.setState({ isUploadModalOpen: true }); });
    expect(alertText()).toBeUndefined();
  });
});
