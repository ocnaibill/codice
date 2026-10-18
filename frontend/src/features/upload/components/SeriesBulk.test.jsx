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
let posts;
const file = (name, folder) => {
  const f = new File([new Uint8Array(64)], name, { lastModified: ++stamp });
  if (folder) Object.defineProperty(f, 'webkitRelativePath', { value: `${folder}/${name}` });
  return f;
};
const body = document.body;
const picker = (label) => [...body.querySelectorAll('input[type="file"]')].find((i) => i.getAttribute('aria-label') === label);
const choose = async (label, ...files) => {
  await act(async () => {
    Object.defineProperty(picker(label), 'files', { value: files, configurable: true });
    picker(label).dispatchEvent(new Event('change', { bubbles: true }));
  });
};
const tableRows = () => [...body.querySelectorAll('tbody tr')];
const field = (label) => body.querySelector(`input[aria-label="${label}"]`);
const typeInto = async (input, value) => {
  await act(async () => {
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
    set.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
};
const buttonLike = (re) => [...body.querySelectorAll('button')].find((b) => re.test(b.textContent.trim()));
const click = async (re) => { await act(async () => { buttonLike(re).click(); }); await flush(); };
const seriesName = () => body.querySelector('input[type="text"]').value;
const open = async () => {
  useGlobalStore.setState({ isUploadModalOpen: true });
  view = await mount(<UploadModal />);
  await act(async () => { buttonLike(/^Série em massa$/).click(); });
};
const answer = async (n, how, value) => {
  await act(async () => { if (how === 'ok') posts[n].resolve({ data: value ?? {} }); else posts[n].reject(value); });
  await flush();
};
const sent = (n) => {
  const form = posts[n].body;
  return { keys: [...form.keys()], get: (k) => form.get(k) };
};

beforeEach(() => {
  vi.clearAllMocks();
  posts = [];
  api.post.mockImplementation((url, form, options) => new Promise((resolve, reject) => { posts.push({ url, body: form, options, resolve, reject }); }));
  useToasts.getState().clear();
  useGlobalStore.setState({ isUploadModalOpen: false });
});
afterEach(() => view.unmount());

const NAMES = ['Jujutsu Kaisen Modelo - Cap 1.cbz', 'Jujutsu Kaisen Modelo - Cap 2.cbz', 'Jujutsu Kaisen Modelo - Cap 4.cbz'];

describe('the Série em massa tab', () => {
  it('is a second tab of the same dialog, and the first one is still there', async () => {
    await open();
    expect(body.querySelector('[role="tablist"]')).not.toBeNull();
    expect(picker('Escolher arquivos')).not.toBeNull();
    expect(picker('Escolher pasta').hasAttribute('webkitdirectory')).toBe(true);
    await act(async () => { buttonLike(/^Arquivos$/).click(); });
    expect(body.querySelector('label[data-dragging]')).not.toBeNull();
  });

  it('reads the series, the numbers and the titles from the names, and says what is missing', async () => {
    await open();
    await choose('Escolher arquivos', ...NAMES.map((n) => file(n)));
    expect(seriesName()).toBe('Jujutsu Kaisen Modelo');
    expect(tableRows()).toHaveLength(3);
    expect(field('Número de ' + NAMES[2]).value).toBe('4');
    expect(field('Título de ' + NAMES[0]).value).toBe('Jujutsu Kaisen Modelo - Cap. 1');
    expect(body.querySelector('[aria-label="Resumo"]').textContent).toContain('faltam 3');
    expect(body.querySelector('[aria-label="Resumo"]').textContent).toContain('de 1 a 4');
    expect(buttonLike(/^Enviar 3/).disabled).toBe(false);
  });

  it('suggests the series from the folder when the names do not say it', async () => {
    await open();
    await choose('Escolher pasta', file('Cap 1.cbz', 'Jujutsu Kaisen'), file('Cap 2.cbz', 'Jujutsu Kaisen'));
    expect(seriesName()).toBe('Jujutsu Kaisen');
  });

  it('leaves out what a series does not take, and says so', async () => {
    await open();
    await choose('Escolher arquivos', file('S Cap 1.cbz'), file('notas.txt'));
    expect(tableRows()).toHaveLength(1);
    expect(body.querySelector('[role="alert"]').textContent).toContain('não é CBZ, CBR nem PDF');
  });

  it('does not let a repeated number go, and goes once the number is fixed', async () => {
    await open();
    await choose('Escolher arquivos', file('S Cap 1.cbz'), file('S Cap 2.cbz'), file('S Cap 2 (b).cbz'));
    const send = () => buttonLike(/^Enviar/);
    expect(send().disabled).toBe(true);
    expect(send().textContent).toContain('corrigir 2 antes');
    expect(tableRows().filter((r) => r.getAttribute('data-situation') === 'repeated')).toHaveLength(2);
    await typeInto(field('Número de S Cap 2 (b).cbz'), '3');
    expect(send().disabled).toBe(false);
    expect(field('Título de S Cap 2 (b).cbz').value).toBe('S - Cap. 3');
  });

  it('keeps a title the person wrote when the series is renamed', async () => {
    await open();
    await choose('Escolher arquivos', file('S Cap 1.cbz'), file('S Cap 2.cbz'));
    await typeInto(field('Título de S Cap 1.cbz'), 'Prólogo');
    await typeInto(body.querySelector('input[type="text"]'), 'Outra');
    expect(field('Título de S Cap 1.cbz').value).toBe('Prólogo');
    expect(field('Título de S Cap 2.cbz').value).toBe('Outra - Cap. 2');
  });

  it('sends the place before the file, three at a time, and tells what happened', async () => {
    await open();
    await choose('Escolher arquivos', ...['A', 'B', 'C', 'D'].map((_, i) => file(`Serie Cap ${i + 1}.cbz`)));
    await act(async () => { buttonLike(/^Enviar 4/).click(); });
    await flush();
    expect(posts).toHaveLength(3); // three at once, the fourth waits
    const first = sent(0);
    expect(first.keys.at(-1)).toBe('document'); // the server reads the fields only before the file
    expect([first.get('series'), first.get('series_index'), first.get('unit'), first.get('comic_kind'), first.get('title')])
      .toEqual(['Serie', '1', 'chapter', 'manga', 'Serie - Cap. 1']);

    await answer(0, 'ok', { work_id: 1, collection_id: 7 });
    expect(posts).toHaveLength(4);
    await answer(1, 'ok');
    await answer(2, 'reject', { response: { status: 409, data: { error: 'in_series', title: 'Serie - Cap. 3', work_id: 3 } } });
    await answer(3, 'reject', { response: { status: 500, data: 'x' } });

    const statuses = tableRows().map((r) => r.getAttribute('data-status'));
    expect(statuses).toEqual(['done', 'done', 'skipped', 'error']);
    expect(body.textContent).toContain('Já existe na série como “Serie - Cap. 3”: foi pulado.');
    expect(refreshLibrary).toHaveBeenCalledTimes(1);
    const toast = useToasts.getState().items[0];
    expect(JSON.stringify(toast)).toContain('2 capítulos enviados');
    expect(buttonLike(/^Abrir a série/)).toBeTruthy();
    expect(buttonLike(/^Tentar de novo 1/)).toBeTruthy();
  });

  it('opens the series it filled', async () => {
    const openCollection = vi.fn();
    useGlobalStore.setState({ openCollection });
    await open();
    await choose('Escolher arquivos', file('S Cap 1.cbz'));
    await click(/^Enviar 1/);
    await answer(0, 'ok', { work_id: 1, collection_id: 42 });
    await click(/^Abrir a série/);
    expect(openCollection).toHaveBeenCalledWith(42);
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
  });

  it('stops the rest, letting the ones going finish', async () => {
    await open();
    await choose('Escolher arquivos', ...[1, 2, 3, 4, 5].map((n) => file(`S Cap ${n}.cbz`)));
    await click(/^Enviar 5/);
    expect(posts).toHaveLength(3);
    await click(/^Parar o resto/);
    await answer(0, 'ok');
    await answer(1, 'ok');
    await answer(2, 'ok');
    expect(posts).toHaveLength(3);
    expect(tableRows().map((r) => r.getAttribute('data-status'))).toEqual(['done', 'done', 'done', 'cancelled', 'cancelled']);
    expect(buttonLike(/^Tentar de novo 2/)).toBeTruthy();
  });

  it('does not let the dialog close while it sends', async () => {
    await open();
    await choose('Escolher arquivos', file('S Cap 1.cbz'));
    await click(/^Enviar 1/);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(true);
    await answer(0, 'ok');
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(useGlobalStore.getState().isUploadModalOpen).toBe(false);
  });
});
