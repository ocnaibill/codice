import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { act } from 'react';
import { OcrTab } from './OcrTab';

let view;
const settings = (over = {}) => ({
  enabled: false, language: 'por+eng', available: true, engine: 'tesseract', engineVersion: '5.5.0',
  languages: ['eng', 'por', 'spa'], state: 'idle', error: '', ...over,
});
const scan = (over = {}) => ({
  workId: 1, title: 'Escaneado', fileId: 4, pageCount: 5, pagesWithoutText: [1, 2, 3, 4, 5], read: 0, failed: 0, state: '', languages: [], ...over,
});

async function open({ state = settings(), files = [], isOwner = true } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/ocr/settings') return { data: state };
    if (url === '/admin/ocr') return { data: { data: files } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: state });
  api.post.mockResolvedValue({ data: { queued: true } });
  view = await mount(<OcrTab isOwner={isOwner} />);
}
const box = (label) => [...document.body.querySelectorAll('label')].find((l) => l.textContent.includes(label))?.querySelector('input');
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('OcrTab: the settings', () => {
  it('explains what it does, which engine is there and what it is doing', async () => {
    await open();
    const text = view.text();
    expect(text).toContain('Leitura de páginas escaneadas (OCR)');
    expect(text).toContain('O arquivo original não muda');
    expect(text).toContain('pode ter erros');
    expect(text).toContain('Motor: tesseract 5.5.0');
    expect(text).toContain('Parado, esperando páginas para ler.');
  });

  it('is off until the owner turns it on, and turning it on keeps the language', async () => {
    await open();
    expect(box('Ler as páginas escaneadas').checked).toBe(false);
    await view.click(box('Ler as páginas escaneadas'));
    expect(api.put).toHaveBeenCalledWith('/admin/ocr/settings', { enabled: true, language: 'por+eng' });
  });

  it('turns off whatever the engine is doing', async () => {
    await open({ state: settings({ enabled: true, state: 'working' }) });
    expect(view.text()).toContain('Lendo páginas agora.');
    expect(box('Ler as páginas escaneadas').checked).toBe(true);
    await view.click(box('Ler as páginas escaneadas'));
    expect(api.put).toHaveBeenCalledWith('/admin/ocr/settings', { enabled: false, language: 'por+eng' });
  });

  it('says the service is not running, and then it cannot be turned on', async () => {
    await open({ state: settings({ available: false, engine: '', engineVersion: '', languages: [] }) });
    expect(view.text()).toContain('O serviço de OCR não está em execução');
    expect(view.text()).not.toContain('Motor:');
    expect(box('Ler as páginas escaneadas').disabled).toBe(true);
  });

  it('lets it be turned off when the service is gone while it was on', async () => {
    await open({ state: settings({ enabled: true, available: false, languages: [] }) });
    expect(box('Ler as páginas escaneadas').disabled).toBe(false);
  });

  it('reports an engine that could not work', async () => {
    await open({ state: settings({ state: 'error', error: 'RuntimeError: 5 pages in a row could not be read' }) });
    expect(view.text()).toContain('O serviço de OCR não conseguiu trabalhar.');
    expect(view.text()).toContain('5 pages in a row');
  });

  it('offers the languages the engine has, by name, with the ones chosen marked', async () => {
    await open();
    expect(box('Português').checked).toBe(true);
    expect(box('Inglês').checked).toBe(true);
    expect(box('Espanhol').checked).toBe(false);
    expect(view.text()).toContain('Um PDF que declara o idioma é lido nele');
  });

  it('adds a language at the end and takes one away, keeping the order of the others', async () => {
    await open();
    await view.click(box('Espanhol'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/ocr/settings', { enabled: false, language: 'por+eng+spa' });
    await view.click(box('Inglês'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/ocr/settings', { enabled: false, language: 'por' });
  });

  it('never lets the last language be taken away', async () => {
    await open({ state: settings({ language: 'por' }) });
    expect(box('Português').disabled).toBe(true);
    expect(box('Inglês').disabled).toBe(false);
  });

  it('keeps the language chosen with the engine turned on when another is added', async () => {
    await open({ state: settings({ enabled: true, language: 'eng' }) });
    await view.click(box('Português'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/ocr/settings', { enabled: true, language: 'eng+por' });
  });

  it('shows the language chosen, without the means to change it, while the engine is not there', async () => {
    await open({ state: settings({ available: false, languages: [], language: 'por+eng' }) });
    expect(box('Português').checked).toBe(true);
    expect(box('Português').disabled).toBe(true);
    expect(box('Inglês').disabled).toBe(true);
  });

  it('says what the server said when it refuses', async () => {
    await open();
    api.put.mockRejectedValue({ response: { status: 409, data: 'O serviço de OCR não está em execução.' } });
    await view.click(box('Ler as páginas escaneadas'));
    expect(view.text()).toContain('O serviço de OCR não está em execução.');
  });

  it('is only to be read by the staff that is not the owner', async () => {
    await open({ state: settings({ enabled: true }), isOwner: false });
    expect(view.text()).toContain('O OCR está ligado. Quem decide é o dono do acervo.');
    expect(box('Ler as páginas escaneadas')).toBeUndefined();
    expect(box('Português')).toBeUndefined();
    view.unmount();
    await open({ isOwner: false });
    expect(view.text()).toContain('O OCR está desligado.');
  });

  it('says when the settings could not be loaded', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/ocr/settings') throw new Error('boom');
      return { data: { data: [] } };
    });
    view = await mount(<OcrTab isOwner />);
    expect(view.text()).toContain('Não foi possível carregar a configuração do OCR.');
  });
});

describe('OcrTab: the scans', () => {
  it('says there are none', async () => {
    await open();
    expect(view.text()).toContain('Nenhum PDF precisa de OCR.');
  });

  it('lists each scan with how much of it is image and how far it is', async () => {
    await open({ state: settings({ enabled: true }), files: [
      scan({ title: 'Todo escaneado', read: 5, languages: ['por+eng'] }),
      scan({ workId: 2, title: 'Misto', fileId: 6, pageCount: 10, pagesWithoutText: [3, 4], state: 'queued' }),
    ] });
    const text = view.text();
    expect(text).toContain('Todo escaneado');
    expect(text).toContain('Todas as 5 páginas são imagem');
    expect(text).toContain('As 5 páginas foram lidas (Português e Inglês)');
    expect(text).toContain('2 de 10 páginas sem texto');
    expect(text).toContain('Na fila para ser lido');
  });

  it('says it waits for the owner to turn OCR on', async () => {
    await open({ files: [scan()] });
    expect(view.text()).toContain('5 páginas sem texto, esperando o OCR ser ligado');
  });

  it('shows the progress of a scan that is being read, and only then', async () => {
    await open({ state: settings({ enabled: true }), files: [
      scan({ title: 'Lendo', state: 'reading', read: 2 }),
      scan({ workId: 2, title: 'Pronto', fileId: 5, read: 5 }),
      scan({ workId: 3, title: 'Parado', fileId: 6, read: 2 }), // some read, nothing running: waiting its turn
    ] });
    const bars = [...document.body.querySelectorAll('[role="progressbar"]')];
    expect(bars.map((b) => b.getAttribute('aria-valuenow'))).toEqual(['40', '40']);
    expect(view.text()).toContain('Lendo: 2 de 5 páginas');
    expect(view.text()).toContain('2 de 5 páginas lidas; esperando a vez');
  });

  it('counts the pages that failed as handled in the progress', async () => {
    await open({ state: settings({ enabled: true }), files: [scan({ state: 'reading', read: 2, failed: 1 })] });
    expect(document.body.querySelector('[role="progressbar"]').getAttribute('aria-valuenow')).toBe('60');
  });

  it('offers to try the pages that failed again, and asks for it', async () => {
    await open({ state: settings({ enabled: true }), files: [scan({ read: 3, failed: 2 }), scan({ workId: 2, title: 'Uma só', fileId: 9, read: 4, failed: 1 })] });
    expect(view.text()).toContain('3 de 5 páginas lidas; 2 não puderam ser lidas');
    expect(view.buttonMatching(/Tentar de novo as 2 páginas que falharam/)).toBeTruthy();
    expect(view.buttonMatching(/Tentar de novo a página que falhou/)).toBeTruthy();
    await view.click(view.buttonMatching(/Tentar de novo as 2 páginas/));
    expect(api.post).toHaveBeenCalledWith('/admin/works/1/ocr/retry');
  });

  it('does not offer it while OCR is off, or while the work is waiting or being read', async () => {
    await open({ files: [scan({ read: 3, failed: 2 })] });
    expect(view.buttonMatching(/Tentar de novo/)).toBeUndefined();
    view.unmount();
    await open({ state: settings({ enabled: true }), files: [scan({ read: 3, failed: 2, state: 'queued' }), scan({ workId: 2, fileId: 8, read: 3, failed: 2, state: 'reading' })] });
    expect(view.buttonMatching(/Tentar de novo/)).toBeUndefined();
  });

  it('says what the server said when the retry is refused', async () => {
    await open({ state: settings({ enabled: true }), files: [scan({ read: 3, failed: 2 })] });
    api.post.mockRejectedValue({ response: { status: 409, data: 'O OCR está desligado.' } });
    await view.click(view.buttonMatching(/Tentar de novo/));
    expect(view.text()).toContain('O OCR está desligado.');
  });

  it('says when the list could not be loaded', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/ocr') throw new Error('boom');
      return { data: settings() };
    });
    view = await mount(<OcrTab isOwner />);
    expect(view.text()).toContain('Não foi possível carregar a lista.');
  });

  it('keeps asking while OCR is on, because the work is queued a moment after it is turned on', async () => {
    // What the screen got right after the owner turned it on: nothing is queued yet, nothing says "waiting".
    let files = 0;
    let engine = 0;
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/ocr/settings') { engine += 1; return { data: settings({ enabled: true, state: engine < 2 ? 'idle' : 'working' }) }; }
      files += 1;
      return { data: { data: [scan(files < 2 ? {} : { state: 'reading', read: 2 })] } };
    });
    view = await mount(<OcrTab isOwner />);
    expect(view.text()).toContain('5 páginas esperando a vez');
    expect(view.text()).toContain('Parado, esperando páginas para ler.');
    await act(async () => { await new Promise((r) => setTimeout(r, 3300)); });
    await flush();
    expect(view.text()).toContain('Lendo: 2 de 5 páginas');
    expect(view.text()).toContain('Lendo páginas agora.');
  }, 20000);

  it('does not keep asking while OCR is off and nothing is being read', async () => {
    let calls = 0;
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/ocr/settings') return { data: settings({ enabled: false }) };
      calls += 1;
      return { data: { data: [scan()] } };
    });
    view = await mount(<OcrTab isOwner />);
    const seen = calls;
    await act(async () => { await new Promise((r) => setTimeout(r, 3300)); });
    expect(calls).toBe(seen);
  }, 20000);

  it.each([['reading', 'Lendo: 1 de 5 páginas'], ['queued', 'Na fila para ser lido']])('asks again by itself while a scan is %s, and stops when it is done (OCR turned off meanwhile)', async (state, said) => {
    let calls = 0;
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/ocr/settings') return { data: settings({ enabled: false }) };
      calls += 1;
      return { data: { data: [scan(calls < 2 ? { state, read: 1 } : { read: 5 })] } };
    });
    view = await mount(<OcrTab isOwner />);
    expect(view.text()).toContain(said);
    await act(async () => { await new Promise((r) => setTimeout(r, 3300)); });
    await flush();
    expect(view.text()).toContain('As 5 páginas foram lidas');
    const seen = calls;
    await act(async () => { await new Promise((r) => setTimeout(r, 3300)); });
    expect(calls).toBe(seen); // nothing is waiting any more: it does not ask again
  }, 20000);
});
