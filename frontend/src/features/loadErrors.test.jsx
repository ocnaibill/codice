// The same for the screens of the rest of the app (#77): when what a screen was loading does not come, it says so and offers
// to try again, and trying again asks the server again. (The admin tabs are tried in admin/components/loadErrors.test.jsx.)
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

vi.mock('../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn(), patch: vi.fn() },
  authenticatedUrl: (u) => u,
}));

import { api } from '../lib/api';
import { useGlobalStore } from '../store/useGlobalStore';
import { mount } from './admin/testUtils';
import { NotesPage } from './notes/NotesPage';
import { NotesPanel } from './reader/components/NotesPanel';
import { SearchPage } from './search/SearchPage';
import { PreferencesModal } from '../components/layout/PreferencesModal';
import { WorkSheet } from './reader/components/WorkSheet';
import { EditBookModal } from './library/components/EditBookModal';
import { JoinVersionsDialog } from './reader/components/JoinVersionsDialog';

let view;
let failing;
let failure;
beforeEach(() => {
  vi.clearAllMocks();
  failing = true;
  failure = new Error('fora do ar');
  useGlobalStore.setState({ sheetWorkId: 7 });
});
afterEach(() => view?.unmount());

function serve(payload) {
  api.get.mockImplementation(async () => {
    if (failing) throw failure;
    return { data: payload };
  });
}
const alerts = () => [...document.body.querySelectorAll('[role="alert"]')];
const said = (message) => alerts().filter((a) => a.textContent.includes(message)).length;

const work = { id: 7, title: 'Duna', author: 'Frank Herbert', format: 'epub', tags: [], editions: [], authors: [], metadata: {} };
const CASES = [
  ['as anotações (a tela)', () => <NotesPage />, 'Não foi possível carregar as anotações.', { data: [], total: 0 }],
  ['as anotações (o painel do leitor)', () => <NotesPanel workId={7} fileId={4} getLocator={() => null} onOpenAt={vi.fn()} onClose={vi.fn()} />, 'Não foi possível carregar as anotações.', { data: [], total: 0 }],
  ['as obras da busca', () => <SearchPage query="duna" />, 'Não foi possível buscar obras.', { data: [], total: 0 }],
  ['as passagens da busca', () => <SearchPage query="duna" />, 'Não foi possível buscar passagens.', { data: [], total: 0 }],
  ['as anotações da busca', () => <SearchPage query="duna" />, 'Não foi possível buscar anotações.', { data: [], total: 0 }],
  ['as preferências', () => <PreferencesModal onClose={vi.fn()} />, 'Não foi possível carregar as preferências.', { nameOrder: 'given_first', libraryNameOrder: 'given_first' }],
  ['a ficha da obra', () => <WorkSheet />, 'Não foi possível abrir esta obra.', work],
  ['a edição da obra', () => <EditBookModal workId={7} tab="edit" onClose={vi.fn()} />, 'Não foi possível abrir esta obra.', work],
];

describe('the screens that cannot load what they show', () => {
  it.each(CASES)('says it could not load %s, and offers to try again', async (_name, element, message, payload) => {
    serve(payload);
    view = await mount(element());
    expect(said(message), message).toBeGreaterThan(0);
    expect([...document.body.querySelectorAll('[role="alert"] button')].some((b) => b.textContent === 'Tentar de novo')).toBe(true);
  });

  it.each(CASES)('asks again when it is told to, and the message goes with %s', async (_name, element, message, payload) => {
    serve(payload);
    view = await mount(element());
    const before = api.get.mock.calls.length;
    const count = said(message);
    failing = false;
    const owner = alerts().find((a) => a.textContent.includes(message));
    await view.click([...owner.querySelectorAll('button')].find((b) => b.textContent === 'Tentar de novo'));
    expect(api.get.mock.calls.length).toBeGreaterThan(before);
    expect(said(message), `${message} went`).toBe(count - 1);
  });

  it('says it could not search the works to join, and searches again', async () => {
    serve({ data: [] });
    view = await mount(<JoinVersionsDialog work={{ id: 7, title: 'Duna', author: 'Frank Herbert' }} onClose={vi.fn()} onJoined={vi.fn()} />);
    const input = document.body.querySelector('input');
    await act(async () => {
      const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      set.call(input, 'dun');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => { await new Promise((r) => setTimeout(r, 400)); }); // the search waits a moment after typing
    await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
    expect(said('Não foi possível procurar.')).toBe(1);
    failing = false;
    const before = api.get.mock.calls.length;
    await view.click([...alerts()[0].querySelectorAll('button')].find((b) => b.textContent === 'Tentar de novo'));
    expect(api.get.mock.calls.length).toBeGreaterThan(before);
    expect(said('Não foi possível procurar.')).toBe(0);
  });
});

describe('and when the server says the person may not see it', () => {
  it.each(CASES)('says so, and does not offer to try again, for %s', async (_name, element, message, payload) => {
    failure = { response: { status: 403, data: 'Forbidden' } };
    serve(payload);
    view = await mount(element());
    const notes = [...document.body.querySelectorAll('[role="note"]')].map((n) => n.textContent);
    expect(notes.some((n) => n.includes('Você não tem permissão para ver isto')), message).toBe(true);
    expect([...document.body.querySelectorAll('[role="alert"]')].some((a) => a.textContent.includes(message)), `${message} is not said as a failure`).toBe(false);
  });
});
