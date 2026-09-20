import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
// The viewer is not what is under test: a button that says "I reached the end".
vi.mock('./viewers/PdfViewer', () => ({
  default: ({ onProgress }) => (
    <button onClick={() => onProgress({ type: 'pdf', page: 9 }, { percent: 100, completed: true })}>reach the end</button>
  ),
}));

import { api } from '../../../lib/api';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Reader } from './Reader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const otherInProgress = { id: 11, format: 'epub', availability: 'available', started: true, completed: false, percentComplete: 30, url: '/f/11' };
const detail = (over = {}) => ({
  id: 7, title: 'Duna', author: 'Frank Herbert', fileId: 10, fileUrl: '/f/10', format: 'pdf', finished: false,
  editions: [
    { id: 1, language: 'pt', files: [{ id: 10, format: 'pdf', availability: 'available', started: true, completed: true, url: '/f/10' }] },
    { id: 2, language: 'en', files: [otherInProgress] },
  ],
  ...over,
});

let container;
let root;
let workDetail;
let progress;

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim().includes(text));
const dialog = () => container.querySelector('[role=dialog]');

async function open({ alreadyCompleted = false, work = detail(), equivalent } = {}) {
  workDetail = work;
  progress = { revision: 1, position: '4', completed: alreadyCompleted };
  api.get.mockImplementation(async (url) => {
    if (url === '/works/7') return { data: workDetail };
    if (url === '/progress/files/10') return { data: progress };
    if (url === '/progress/files/10/equivalent' && equivalent) return { data: equivalent };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockImplementation(async (url) => {
    if (url === '/progress/files/10') return { data: { revision: 2, completed: true } };
    return { data: {} };
  });
  api.post.mockResolvedValue({});
  useGlobalStore.setState({ activeBookId: 7, activeFileId: 10, fromStart: false, seek: null });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { root.render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>); });
  await flush();
  await flush();
}
const reachTheEnd = async () => {
  await act(async () => { button('reach the end').click(); });
  await flush();
  await flush();
};

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  useGlobalStore.getState().closeBook();
});

describe('Reader: finishing a version while another is in progress (DEC-080)', () => {
  it('records the opening of the file', async () => {
    await open();
    expect(api.post).toHaveBeenCalledWith('/progress/files/10/opened');
  });

  it('asks whether the whole work is finished, naming the other version, and marks it on yes', async () => {
    await open();
    expect(dialog()).toBeNull();
    await reachTheEnd();
    expect(dialog()).not.toBeNull();
    expect(dialog().textContent).toContain('EPUB (Inglês), 30%');
    await act(async () => { button('Sim, a obra está finalizada').click(); });
    await flush();
    expect(api.put).toHaveBeenCalledWith('/progress/works/7/finished', { finished: true });
    expect(dialog()).toBeNull();
  });

  it('leaves the other version alone on no', async () => {
    await open();
    await reachTheEnd();
    await act(async () => { button('Não, continuar a outra versão depois').click(); });
    expect(dialog()).toBeNull();
    expect(api.put.mock.calls.some(([url]) => url.includes('/finished'))).toBe(false);
  });

  it('does not ask when nothing else is in progress', async () => {
    await open({ work: detail({ editions: [{ id: 1, language: 'pt', files: [{ id: 10, format: 'pdf', availability: 'available', started: true, completed: true, url: '/f/10' }, { ...otherInProgress, started: false }] }] }) });
    await reachTheEnd();
    expect(dialog()).toBeNull();
  });

  it('does not ask when the whole work is already marked finished', async () => {
    await open({ work: detail({ finished: true }) });
    await reachTheEnd();
    expect(dialog()).toBeNull();
  });

  it('does not ask again for a file that was already finished when it was opened, or twice for one save after another', async () => {
    await open({ alreadyCompleted: true });
    await reachTheEnd();
    expect(dialog()).toBeNull(); // it saves its last page again: that is not finishing

    act(() => root.unmount());
    root = createRoot(container);
    await open();
    await reachTheEnd();
    await act(async () => { button('Não, continuar a outra versão depois').click(); });
    await reachTheEnd();
    expect(dialog()).toBeNull(); // asked once
  });
});

describe('Reader: offering an equivalent position in another version (RF-042)', () => {
  const inProgressWork = (over = {}) =>
    detail({ inProgress: true, continue: { fileId: 11, format: 'epub', language: 'en', percentComplete: 30, completed: false }, ...over });
  const equivalentDialog = () => container.querySelector('[aria-label="Continuar de onde parou?"]');
  const found = {
    status: 'found', sourceExcerpt: 'Um trecho de origem.',
    candidates: [{ method: 'text', confidence: 'high', precision: 'passage', section: 'Capítulo 3', excerpt: 'Um trecho achado.', locator: { type: 'epub', href: 'c3.xhtml' } }],
  };

  it('asks the destination file for a match against the other in-progress version', async () => {
    await open({ work: inProgressWork(), equivalent: found });
    expect(api.get).toHaveBeenCalledWith('/progress/files/10/equivalent', { params: { from: 11 } });
  });

  it('offers to jump when a place was found, and accepting records it and seeks there', async () => {
    await open({ work: inProgressWork(), equivalent: found });
    expect(equivalentDialog()).not.toBeNull();
    expect(equivalentDialog().textContent).toContain('Um trecho de origem.');
    expect(equivalentDialog().textContent).toContain('Um trecho achado.');

    await act(async () => { button('Continuar daqui').click(); });
    expect(api.post).toHaveBeenCalledWith('/progress/files/10/equivalent/accept', {
      sourceFileId: 11, locator: { type: 'epub', href: 'c3.xhtml' }, method: 'text', confidence: 'high', precision: 'passage',
    });
    expect(equivalentDialog()).toBeNull();
    const state = useGlobalStore.getState();
    expect(state.activeBookId).toBe(7);
    expect(state.activeFileId).toBe(10);
    expect(state.seek?.locator).toEqual({ type: 'epub', href: 'c3.xhtml' });
  });

  it('declining leaves the file at its own saved position, with no acceptance recorded', async () => {
    await open({ work: inProgressWork(), equivalent: found });
    await act(async () => { button('Não, abrir minha posição').click(); });
    expect(equivalentDialog()).toBeNull();
    expect(api.post.mock.calls.some(([url]) => url.includes('/equivalent/accept'))).toBe(false);
    expect(useGlobalStore.getState().seek).toBeNull();
  });

  it('lets the person choose among more than one place when ambiguous', async () => {
    const ambiguous = {
      status: 'ambiguous',
      candidates: [
        { method: 'text', confidence: 'medium', precision: 'passage', excerpt: 'Primeira opção.', locator: { type: 'epub', href: 'a.xhtml' } },
        { method: 'anchors', confidence: 'medium', precision: 'passage', excerpt: 'Segunda opção.', locator: { type: 'epub', href: 'b.xhtml' } },
      ],
    };
    await open({ work: inProgressWork(), equivalent: ambiguous });
    const radios = [...container.querySelectorAll('input[type=radio]')];
    expect(radios.length).toBe(2);
    await act(async () => { radios[1].click(); });
    await act(async () => { button('Continuar daqui').click(); });
    expect(api.post).toHaveBeenCalledWith('/progress/files/10/equivalent/accept', expect.objectContaining({ locator: { type: 'epub', href: 'b.xhtml' } }));
  });

  it('shows nothing when no equivalent place was found', async () => {
    await open({ work: inProgressWork(), equivalent: { status: 'not_found', candidates: [] } });
    expect(equivalentDialog()).toBeNull();
  });

  it('does not ask when there is no other version in progress', async () => {
    await open({ work: detail() });
    expect(api.get.mock.calls.some(([url]) => url.includes('/equivalent'))).toBe(false);
    expect(equivalentDialog()).toBeNull();
  });

  it('does not ask when the other version exists but is not the one in progress', async () => {
    // A "continue" file can be present and finished (DEC-079/080): that is not an invitation to jump.
    await open({ work: inProgressWork({ inProgress: false }) });
    expect(api.get.mock.calls.some(([url]) => url.includes('/equivalent'))).toBe(false);
    expect(equivalentDialog()).toBeNull();
  });
});
