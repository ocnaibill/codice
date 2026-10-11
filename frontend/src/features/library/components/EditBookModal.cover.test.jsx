import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The cut itself needs a canvas, which the test browser does not have; the geometry is tested on its own.
vi.mock('../coverCrop', async (importOriginal) => ({ ...(await importOriginal()), cutToBlob: vi.fn() }));

import { api } from '../../../lib/api';
import { cutToBlob } from '../coverCrop';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { EditBookModal } from './EditBookModal';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let metadata;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === text);
const click = async (el) => { await act(async () => { el.click(); }); await flush(); };

class FakeImage {
  set src(value) {
    this._src = value;
    queueMicrotask(() => { this.naturalWidth = 600; this.naturalHeight = 900; this.onload?.(); });
  }
  get src() { return this._src; }
}

async function open({ coverUrl = '/covers/duna.jpg', sources = {}, locks = {} } = {}) {
  metadata = { firstAuthor: 'Frank Herbert', locks, sources, alternativeTitles: [] };
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: { id: 7, title: 'Duna', author: 'Frank Herbert', tags: [], format: 'epub', coverUrl, editions: [], metadata } };
    if (url === '/works/7/candidates') return { data: { data: [] } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({ data: { url: '/covers/cover_manual_7_1.jpg' } });
  api.delete.mockResolvedValue({ data: { url: '/covers/duna.jpg' } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><EditBookModal workId={7} tab="edit" onClose={vi.fn()} /></QueryClientProvider>); });
  await flush();
  await flush();
}

async function choosePicture(type = 'image/png', size = 10) {
  const input = document.querySelector('input[type="file"]');
  const file = new File([new Uint8Array(size)], 'capa.png', { type });
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  await act(async () => { input.dispatchEvent(new Event('change', { bubbles: true })); });
  await flush();
}

const coverPreview = () => document.querySelector('form img');
const savedBody = () => api.put.mock.calls.find(([url]) => url === '/works/7')[1];

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('Image', FakeImage);
  URL.createObjectURL = vi.fn(() => 'blob:picture');
  URL.revokeObjectURL = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  useGlobalStore.setState({ sheetWorkId: 7 });
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
  useGlobalStore.setState({ sheetWorkId: null });
});

describe('EditBookModal: the cover chosen by hand (DEC-173)', () => {
  it('shows the cover the work has and the way to change it, with no way back while it is the file’s', async () => {
    await open();
    expect(coverPreview().getAttribute('src')).toBe('/covers/duna.jpg');
    expect(document.body.textContent).toContain('A que veio do arquivo');
    expect(button('Trocar capa…')).toBeTruthy();
    expect(button('Voltar à capa do arquivo')).toBeUndefined();
  });

  it('says it was chosen, and offers the way back, when it was', async () => {
    await open({ coverUrl: '/covers/cover_manual_7_1.jpg', sources: { cover: 'manual' }, locks: { cover: true } });
    expect(document.body.textContent).toContain('Escolhida por você');
    expect(button('Voltar à capa do arquivo')).toBeTruthy();
  });

  it('cuts the picture in a dialog and sends the cut at once, not with the form', async () => {
    const cut = new Blob(['jpeg'], { type: 'image/jpeg' });
    cutToBlob.mockResolvedValue(cut);
    await open();
    await choosePicture();
    expect(document.querySelector('[role="dialog"][aria-label="Recortar a capa"]')).toBeTruthy();
    await click(button('Usar esta capa'));
    const [url, form] = api.put.mock.calls.find(([u]) => u === '/works/7/cover');
    expect(url).toBe('/works/7/cover');
    expect(form.get('image')).toBeInstanceOf(Blob);
    expect(form.get('image').size).toBe(cut.size);
    expect(document.querySelector('[role="dialog"][aria-label="Recortar a capa"]')).toBeNull();
    expect(coverPreview().getAttribute('src')).toBe('/covers/cover_manual_7_1.jpg');
    expect(document.body.textContent).toContain('Escolhida por você');
    expect(api.put.mock.calls.some(([u]) => u === '/works/7')).toBe(false);
  });

  it('does not take the lock back when the form is saved afterwards', async () => {
    cutToBlob.mockResolvedValue(new Blob(['jpeg']));
    await open();
    await choosePicture();
    await click(button('Usar esta capa'));
    api.put.mockResolvedValueOnce({});
    await click(button('Salvar'));
    // The form was opened with the cover free; what it sends is what the server now has.
    expect(savedBody().cover_lock).toBe(true);
  });

  it('goes back to the cover of the file, and the lock goes with it', async () => {
    await open({ coverUrl: '/covers/cover_manual_7_1.jpg', sources: { cover: 'manual' }, locks: { cover: true } });
    await click(button('Voltar à capa do arquivo'));
    expect(api.delete).toHaveBeenCalledWith('/works/7/cover');
    expect(coverPreview().getAttribute('src')).toBe('/covers/duna.jpg');
    expect(button('Voltar à capa do arquivo')).toBeUndefined();
    api.put.mockResolvedValueOnce({});
    await click(button('Salvar'));
    expect(savedBody().cover_lock).toBe(false);
  });

  it('refuses what is not a picture the server takes, before opening the dialog', async () => {
    await open();
    await choosePicture('application/pdf');
    expect(document.querySelector('[role="dialog"][aria-label="Recortar a capa"]')).toBeNull();
    expect(document.body.textContent).toContain('JPEG, PNG ou WebP');
    await choosePicture('image/png', 21 * 1024 * 1024);
    expect(document.querySelector('[role="dialog"][aria-label="Recortar a capa"]')).toBeNull();
    expect(document.body.textContent).toContain('mais de 20 MB');
  });

  it('keeps the dialog open, with the reason, when the server refuses the cover', async () => {
    cutToBlob.mockResolvedValue(new Blob(['jpeg']));
    await open();
    api.put.mockRejectedValueOnce({ response: { status: 413, data: 'A capa tem até 2 MB.\n' } });
    await choosePicture();
    await click(button('Usar esta capa'));
    const dialog = document.querySelector('[role="dialog"][aria-label="Recortar a capa"]');
    expect(dialog).toBeTruthy();
    expect(dialog.textContent).toContain('2 MB');
    expect(document.body.textContent).not.toContain('Escolhida por você');
  });
});
