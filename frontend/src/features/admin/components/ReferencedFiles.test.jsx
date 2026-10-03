import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { ReferencedFiles } from './ReferencedFiles';

const file = (id, over = {}) => ({
  fileId: id, workId: id, title: `Livro ${id}`, author: 'Autor', format: 'epub', sizeBytes: 1024 * 1024, rootId: 1, root: '/mnt/livros',
  path: `Livros/livro-${id}.epub`, state: 'ok', availability: 'available', transfer: null, ...over,
});
const summary = { ok: 3, missing: 1, conflict: 1 };
let view;
let listed; // what the server has, filtered by the mock
let transfers = () => ({ data: [] });
let moveAnswer;
let scanJobs = [];

const referencedCalls = () => api.get.mock.calls.filter(([u]) => u === '/admin/storage/referenced');
const lastParams = () => referencedCalls().at(-1)[1].params;
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });

async function open(files, { roots = [{ id: 1, path: '/mnt/livros' }, { id: 2, path: '/mnt/outra' }], cleanups = [], props = {}, total, sum = summary } = {}) {
  listed = files;
  api.get.mockImplementation(async (url, options) => {
    if (url === '/admin/storage/roots') return { data: { roots, managed: '/data' } };
    if (url === '/admin/storage/cleanups') return { data: { data: cleanups } };
    if (url === '/admin/jobs') return { data: { data: scanJobs, counts: {} } };
    if (url === '/admin/storage/referenced') {
      const p = options?.params || {};
      return { data: { data: listed.slice(p.offset || 0, (p.offset || 0) + p.limit), total: total ?? listed.length, summary: sum } };
    }
    if (url === '/admin/storage/transfers') return { data: transfers(options.params.ids.split(',').map(Number)) };
    throw new Error(`unexpected GET ${url}`);
  });
  api.post.mockImplementation(async () => ({ data: moveAnswer }));
  view = await mount(<ReferencedFiles {...props} />);
  await flush();
}
const box = (title) => document.body.querySelector(`input[aria-label="Escolher ${title}"]`);
const pageBox = () => document.body.querySelector('input[aria-label="Escolher os arquivos desta página"]');
const moveButton = () => view.buttonMatching(/^Mover para o gerenciado/);
const type = async (el, value) => {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
};
beforeEach(() => {
  vi.clearAllMocks();
  transfers = () => ({ data: [] });
  moveAnswer = { data: [] };
  scanJobs = [];
});
afterEach(() => view.unmount());

describe('ReferencedFiles: what the library only points at (#15)', () => {
  it('lists each file with where it is, what it is, how big and whether it is still there, and counts them', async () => {
    await open([file(1, { author: 'Frank Herbert', sizeBytes: 2.5 * 1024 * 1024 }), file(2, { format: 'cbz', sizeBytes: null })]);
    const text = view.text();
    expect(text).toContain('Livro 1');
    expect(text).toContain('Frank Herbert');
    expect(text).toContain('/mnt/livros/Livros/livro-1.epub');
    expect(text).toContain('epub'); // shown in capitals by the style
    expect(text).toContain('2,5 MB');
    expect(text).toContain('cbz');
    expect(text).toContain('tamanho desconhecido');
    expect(text).toContain('no disco');
    expect(text).toContain('3 no disco · 1 ausentes do disco · 1 mudaram depois de catalogados');
    expect(text).toContain('copia o arquivo para o armazenamento do Códice, confere a cópia e só então remove o original');
  });

  it('asks for the first page by default, and for each filter it is given', async () => {
    await open([file(1)]);
    expect(lastParams()).toEqual({ limit: 50, offset: 0 });
    await act(async () => {
      const select = document.body.querySelector('select');
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(select, '2');
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
    expect(lastParams()).toEqual({ limit: 50, offset: 0, rootId: '2' });
    expect(view.text()).toContain('(nesta pasta)');
    await view.click(view.button('Ausentes'));
    expect(lastParams()).toEqual({ limit: 50, offset: 0, rootId: '2', state: 'missing' });
    expect(view.button('Ausentes').getAttribute('aria-pressed')).toBe('true');
    await view.click(view.button('Todos'));
    expect(lastParams().state).toBeUndefined();
  });

  it('asks again for the list when told to, because a scan ends after it was asked for', async () => {
    await open([file(1)]);
    const before = referencedCalls().length;
    await view.click(view.button('Atualizar'));
    expect(referencedCalls().length).toBe(before + 1);
    expect(lastParams()).toEqual({ limit: 50, offset: 0 });
  });

  it('searches after a pause, with the text trimmed', async () => {
    await open([file(1)]);
    await type(document.body.querySelector('input[type="search"]'), '  duna ');
    await wait(120);
    expect(lastParams().q).toBeUndefined();
    await wait(300);
    expect(lastParams().q).toBe('duna');
  });

  it('says what is missing when there is nothing to list, by reason', async () => {
    await open([], { roots: [] });
    expect(view.text()).toContain('autorize uma pasta e varra-a');
    view.unmount();
    await open([], { sum: { ok: 0, missing: 0, conflict: 0 } });
    expect(view.text()).toContain('já está no armazenamento gerenciado, ou ainda não foi varrido');
    await view.click(view.button('Mudaram'));
    expect(view.text()).toContain('Nenhum arquivo referenciado com estes filtros.');
  });

  it('does not let a file that is missing or changed be chosen, and says why', async () => {
    await open([file(1), file(2, { state: 'missing' }), file(3, { state: 'conflict' })]);
    expect(box('Livro 1').disabled).toBe(false);
    expect(box('Livro 2').disabled).toBe(true);
    expect(box('Livro 3').disabled).toBe(true);
    expect(view.text()).toContain('ausente do disco');
    expect(view.text()).toContain('Varra a pasta para atualizar.');
    expect(view.text()).toContain('moverá só se voltar a ser o mesmo');
  });

  it('shows a transfer that is waiting, running or failed, and only a failed one may be asked again', async () => {
    await open([
      file(1, { transfer: { jobId: 5, state: 'pending' } }),
      file(2, { transfer: { jobId: 6, state: 'running' } }),
      file(3, { transfer: { jobId: 7, state: 'failed', lastError: 'the original file changed since it was catalogued' } }),
    ]);
    expect(view.text()).toContain('na fila para mover');
    expect(view.text()).toContain('sendo movido…');
    expect(view.text()).toContain('a última tentativa falhou: O original mudou desde que foi catalogado');
    expect(box('Livro 1').disabled).toBe(true);
    expect(box('Livro 2').disabled).toBe(true);
    expect(box('Livro 3').disabled).toBe(false);
  });

  it('asks again by itself while a transfer is waiting', async () => {
    await open([file(1, { transfer: { jobId: 5, state: 'pending' } })]);
    const before = referencedCalls().length;
    await wait(3200);
    expect(referencedCalls().length).toBeGreaterThan(before);
  });

  it('does not ask again by itself when nothing is moving', async () => {
    await open([file(1)]);
    const before = referencedCalls().length;
    await wait(3200);
    expect(referencedCalls().length).toBe(before);
  });
});

describe('ReferencedFiles: while a folder is being scanned', () => {
  const scan = (state) => ({ id: 3, type: 'scan', state });

  it('asks only for scans, and says nothing when none is going on', async () => {
    await open([file(1)]);
    expect(api.get).toHaveBeenCalledWith('/admin/jobs', { params: { type: 'scan', limit: 10 } });
    expect(view.text()).not.toContain('Varredura em andamento');
    expect(referencedCalls().length).toBe(1); // nothing is going on: the list is asked for once
  });

  it('does not keep asking about scans when there is none going on', async () => {
    await open([file(1)]);
    const asks = () => api.get.mock.calls.filter(([u]) => u === '/admin/jobs').length;
    const before = asks();
    await wait(3200);
    expect(asks()).toBe(before);
  });

  it('says a scan is going on while one is waiting, and the list asks again by itself', async () => {
    scanJobs = [scan('pending'), { id: 2, type: 'scan', state: 'succeeded' }];
    await open([file(1)]);
    expect(view.text()).toContain('Varredura em andamento: a lista se atualiza sozinha.');
    const before = referencedCalls().length;
    await wait(3200);
    expect(referencedCalls().length).toBeGreaterThan(before);
  });

  it('says the same while one is running', async () => {
    scanJobs = [scan('running')];
    await open([file(1)]);
    expect(view.text()).toContain('Varredura em andamento');
  });

  it('does not count a scan that ended, failed or was cancelled', async () => {
    scanJobs = [scan('succeeded'), scan('failed'), scan('cancelled')];
    await open([file(1)]);
    expect(view.text()).not.toContain('Varredura em andamento');
    const before = referencedCalls().length;
    await wait(3200);
    expect(referencedCalls().length).toBe(before);
  });

  it('asks for the list once more when the scan ends, and stops saying it is going on', async () => {
    scanJobs = [scan('running')];
    await open([file(1)]);
    expect(view.text()).toContain('Varredura em andamento');
    scanJobs = [scan('succeeded')];
    listed = [file(1), file(2, { title: 'Achado pela varredura' })];
    await wait(2800); // the screen asks about the scans every 2.5 s
    await flush();
    expect(view.text()).not.toContain('Varredura em andamento');
    expect(view.text()).toContain('Achado pela varredura');
  });

  it('works when the scans cannot be asked about: no message, no asking again', async () => {
    await open([file(1)]);
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/jobs') throw new Error('offline');
      if (url === '/admin/storage/roots') return { data: { roots: [], managed: '/d' } };
      if (url === '/admin/storage/cleanups') return { data: { data: [] } };
      return { data: { data: [file(1)], total: 1, summary } };
    });
    expect(view.text()).not.toContain('Varredura em andamento');
  });
});

describe('ReferencedFiles: choosing', () => {
  it('counts what is chosen and how big it is, and the button waits for a choice', async () => {
    await open([file(1, { sizeBytes: 1024 * 1024 }), file(2, { sizeBytes: 3 * 1024 * 1024 })]);
    expect(view.text()).toContain('Nenhum escolhido');
    expect(moveButton().disabled).toBe(true);
    await view.click(box('Livro 1'));
    await view.click(box('Livro 2'));
    expect(view.text()).toContain('2 escolhido(s), 4,0 MB');
    expect(moveButton().disabled).toBe(false);
    await view.click(box('Livro 1'));
    expect(view.text()).toContain('1 escolhido(s), 3,0 MB');
  });

  it('chooses and clears the files of the page that can be moved, and no others', async () => {
    await open([file(1), file(2, { state: 'missing' }), file(3)]);
    expect(view.text()).toContain('Escolher os desta página que podem ser movidos (2)');
    await view.click(pageBox());
    expect(box('Livro 1').checked && box('Livro 3').checked && !box('Livro 2').checked).toBe(true);
    expect(pageBox().checked).toBe(true);
    await view.click(pageBox());
    expect(view.text()).toContain('Nenhum escolhido');
  });

  it('has nothing to choose when no file on the page can be moved', async () => {
    await open([file(1, { state: 'missing' })]);
    expect(pageBox().disabled).toBe(true);
  });

  it('stops at the most it can ask at once, for the one by one and for the whole page', async () => {
    await open([file(1), file(2), file(3), file(4)], { props: { maxSelected: 2 } });
    await view.click(box('Livro 1'));
    await view.click(box('Livro 2'));
    expect(box('Livro 3').disabled).toBe(true);
    expect(box('Livro 1').disabled).toBe(false); // one that is chosen can be taken away
    expect(view.text()).toContain('(o máximo de uma vez é 2)');
    await view.click(box('Livro 1'));
    await view.click(pageBox());
    expect(view.text()).toContain('2 escolhido(s)');
  });

  it('forgets the choice, and goes to the first page, when a filter changes', async () => {
    await open([file(1)]);
    await view.click(box('Livro 1'));
    await view.click(view.button('No disco'));
    expect(view.text()).toContain('Nenhum escolhido');
    expect(box('Livro 1').checked).toBe(false);
  });

  it('goes back to the first page when a filter changes from a later page', async () => {
    await open(Array.from({ length: 120 }, (_, i) => file(i + 1)));
    await view.click(view.button('Próxima'));
    expect(lastParams().offset).toBe(50);
    await view.click(view.button('Ausentes'));
    expect(lastParams()).toMatchObject({ offset: 0, state: 'missing' });
  });

  it('pages through the files fifty at a time', async () => {
    const many = Array.from({ length: 120 }, (_, i) => file(i + 1));
    await open(many);
    expect(view.text()).toContain('1–50 de 120');
    expect(view.button('Anterior').disabled).toBe(true);
    await view.click(view.button('Próxima'));
    expect(lastParams()).toMatchObject({ limit: 50, offset: 50 });
    expect(view.text()).toContain('51–100 de 120');
    await view.click(view.button('Próxima'));
    expect(view.text()).toContain('101–120 de 120');
    expect(view.button('Próxima').disabled).toBe(true);
    await view.click(view.button('Anterior'));
    expect(view.text()).toContain('51–100 de 120');
  });

  it('has no pages for fifty files or fewer', async () => {
    await open(Array.from({ length: 50 }, (_, i) => file(i + 1)));
    expect(view.button('Próxima')).toBeUndefined();
  });
});

describe('ReferencedFiles: moving to the managed storage', () => {
  it('says what happens before asking, including that the original is deleted, and moves nothing if the person backs out', async () => {
    await open([file(1, { sizeBytes: 1024 * 1024 }), file(2, { sizeBytes: 1024 * 1024 })]);
    await view.click(pageBox());
    await view.click(moveButton());
    const dialog = view.dialog().textContent;
    expect(dialog).toContain('Mover 2 arquivos para o armazenamento gerenciado?');
    expect(dialog).toContain('2,0 MB no total');
    expect(dialog).toContain('conferida byte a byte');
    expect(dialog).toContain('Depois disso o original é apagado da pasta de origem');
    expect(dialog).toContain('lista de limpeza pendente');
    expect(dialog).toContain('Nada que seja diferente é sobrescrito');
    await view.click(view.button('Cancelar'));
    expect(api.post).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
    expect(view.text()).toContain('2 escolhido(s)');
  });

  it('says one file in the singular', async () => {
    await open([file(1)]);
    await view.click(box('Livro 1'));
    await view.click(moveButton());
    expect(view.dialog().textContent).toContain('Mover 1 arquivo para o armazenamento gerenciado?');
  });

  it('asks for exactly the files chosen, shows how each ends, and clears the choice', async () => {
    moveAnswer = { data: [{ fileId: 1, jobId: 14 }, { fileId: 2, jobId: 13 }, { fileId: 3, jobId: 12 }, { fileId: 4, jobId: 11 }, { fileId: 5, error: 'not a referenced file that is available' }] };
    transfers = () => ({ data: [
      { jobId: 14, fileId: 1, title: 'Livro 1', format: 'epub', outcome: 'moved' },
      { jobId: 13, fileId: 2, title: 'Livro 2', format: 'pdf', outcome: 'moved_original_kept', origin: '/mnt/livros/Livros/livro-2.pdf', reason: 'the original could not be removed: permission denied' },
      { jobId: 12, fileId: 3, title: 'Livro 3', format: 'epub', outcome: 'failed', error: 'the original file is missing: x' },
      { jobId: 11, fileId: 4, title: 'Livro 4', format: 'cbz', outcome: 'cancelled' },
    ] });
    await open([1, 2, 3, 4, 5].map((i) => file(i)));
    await view.click(pageBox());
    await view.click(moveButton());
    await view.click(view.button('Mover'));
    await flush();
    expect(api.post).toHaveBeenCalledWith('/admin/library/move-to-managed', { fileIds: [1, 2, 3, 4, 5] });
    expect(api.get).toHaveBeenCalledWith('/admin/storage/transfers', { params: { ids: '11,12,13,14' } }); // in order, whatever the order they were queued in

    const region = document.body.querySelector('[aria-label="Resultado da transferência"]').textContent;
    expect(region).toContain('Terminou: 2 movido(s), 3 sem mover.');
    expect(region).toContain('Livro 1 · EPUB');
    expect(region).toContain('movido');
    expect(region).toContain('movido; o original ficou');
    expect(region).toContain('Original em /mnt/livros/Livros/livro-2.pdf: Não foi possível apagar o original: permission denied');
    expect(region).toContain('Um original não pôde ser apagado');
    expect(region).toContain('a cópia gerenciada está correta');
    expect(region).toContain('O original não está mais na pasta de origem.');
    expect(region).toContain('cancelado');
    expect(region).toContain('não entrou na fila');
    expect(region).toContain('Não é um arquivo referenciado disponível');
    expect(view.text()).toContain('Nenhum escolhido');
    expect(moveButton().disabled).toBe(true);
  });

  it('says how many are done while some are still waiting, and does not let the report be closed meanwhile', async () => {
    moveAnswer = { data: [{ fileId: 1, jobId: 11 }, { fileId: 2, jobId: 12 }] };
    transfers = () => ({ data: [{ jobId: 11, fileId: 1, title: 'Livro 1', format: 'epub', outcome: 'moved' }, { jobId: 12, fileId: 2, title: 'Livro 2', format: 'epub', outcome: 'running' }] });
    await open([file(1), file(2)]);
    await view.click(pageBox());
    await view.click(moveButton());
    await view.click(view.button('Mover'));
    await flush();
    const region = () => document.body.querySelector('[aria-label="Resultado da transferência"]');
    expect(region().textContent).toContain('Movendo… 1 de 2 terminados.');
    expect(region().textContent).toContain('movendo…');
    expect([...region().querySelectorAll('button')].find((b) => b.textContent === 'Fechar').disabled).toBe(true);
  });

  it('follows the transfers until they end, asking again by itself', async () => {
    moveAnswer = { data: [{ fileId: 1, jobId: 11 }] };
    let calls = 0;
    transfers = () => ({ data: [{ jobId: 11, fileId: 1, title: 'Livro 1', format: 'epub', outcome: ++calls < 2 ? 'queued' : 'moved' }] });
    await open([file(1)]);
    await view.click(box('Livro 1'));
    await view.click(moveButton());
    await view.click(view.button('Mover'));
    await flush();
    expect(document.body.querySelector('[aria-label="Resultado da transferência"]').textContent).toContain('Movendo… 0 de 1 terminados.');
    await wait(2300);
    const region = document.body.querySelector('[aria-label="Resultado da transferência"]');
    expect(region.textContent).toContain('Terminou: 1 movido(s), 0 sem mover.');
    expect(region.textContent).not.toContain('não puderam ser apagados');
    expect(region.textContent).not.toContain('não pôde ser apagado');
    expect([...region.querySelectorAll('button')].find((b) => b.textContent === 'Fechar').disabled).toBe(false);
    const asks = api.get.mock.calls.filter(([u]) => u === '/admin/storage/transfers').length;
    await wait(2300);
    expect(api.get.mock.calls.filter(([u]) => u === '/admin/storage/transfers').length).toBe(asks); // it stopped asking when all ended
    await view.click([...region.querySelectorAll('button')].find((b) => b.textContent === 'Fechar'));
    expect(document.body.querySelector('[aria-label="Resultado da transferência"]')).toBeNull();
  });

  it('reports a file that did not enter the queue with its reason and no job to follow', async () => {
    moveAnswer = { data: [{ fileId: 1, error: 'another file of this work is being moved: ask again when it ends' }] };
    await open([file(1)]);
    await view.click(box('Livro 1'));
    await view.click(moveButton());
    await view.click(view.button('Mover'));
    await flush();
    expect(api.get.mock.calls.some(([u]) => u === '/admin/storage/transfers')).toBe(false);
    expect(document.body.querySelector('[aria-label="Resultado da transferência"]').textContent).toContain('Outro arquivo desta obra está sendo movido');
  });

  it('says so, and keeps the choice, when the request itself fails', async () => {
    await open([file(1)]);
    api.post.mockRejectedValue({ response: { status: 403, data: 'Forbidden' } });
    await view.click(box('Livro 1'));
    await view.click(moveButton());
    await view.click(view.button('Mover'));
    await flush();
    expect(view.text()).toContain('Você não tem permissão para isso.');
    expect(view.text()).toContain('1 escolhido(s)');
    expect(document.body.querySelector('[aria-label="Resultado da transferência"]')).toBeNull();
  });

  it('says it could not follow the transfers when that fails, without losing what was asked', async () => {
    moveAnswer = { data: [{ fileId: 1, jobId: 11 }] };
    await open([file(1)]);
    api.get.mockImplementation(async (url) => {
      if (url === '/admin/storage/transfers') throw new Error('offline');
      if (url === '/admin/storage/roots') return { data: { roots: [], managed: '/d' } };
      if (url === '/admin/storage/cleanups') return { data: { data: [] } };
      return { data: { data: [file(1)], total: 1, summary } };
    });
    await view.click(box('Livro 1'));
    await view.click(moveButton());
    await view.click(view.button('Mover'));
    await flush();
    const region = document.body.querySelector('[aria-label="Resultado da transferência"]').textContent;
    expect(region).toContain('Não foi possível acompanhar a transferência');
    expect(region).toContain('Livro 1');
  });

  it('refreshes the list when the move was asked for, since what moved leaves it', async () => {
    moveAnswer = { data: [{ fileId: 1, jobId: 11 }] };
    await open([file(1)]);
    const before = referencedCalls().length;
    await view.click(box('Livro 1'));
    await view.click(moveButton());
    await view.click(view.button('Mover'));
    await flush();
    expect(referencedCalls().length).toBeGreaterThan(before);
  });

  it('tells how many originals wait to be deleted, and says where', async () => {
    await open([file(1)], { cleanups: [{ id: 1, path: '/mnt/x.epub', reason: 'read-only' }, { id: 2, path: '/mnt/y.epub', reason: 'busy' }] });
    expect(view.text()).toContain('2 original(is) aguardando para serem apagados');
  });
});
