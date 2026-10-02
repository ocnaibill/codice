import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  authenticatedUrl: (u) => u,
}));
vi.mock('../../lib/download', () => ({ fetchFile: vi.fn(async () => ({ blob: new Blob(['x']), name: 'a.md' })), saveBlob: vi.fn() }));

import { api } from '../../lib/api';
import { useGlobalStore } from '../../store/useGlobalStore';
import { mount, flush } from '../admin/testUtils';
import { NotesPage } from './NotesPage';

const note = (id, over = {}) => ({
  id, kind: 'note', workId: 7, workTitle: 'Duna', workAuthor: 'Frank Herbert', fileId: 70, fileAvailable: true, sourceAvailable: true,
  quote: `Trecho ${id}`, body: '', tags: [], locator: { type: 'pdf', page: 11 }, createdAt: '2026-09-19T10:00:00Z', ...over,
});

let view;
let notes; // what the server has (all of it), filtered by the mock the way the server does
let openBook;
const calls = () => api.get.mock.calls.map(([url]) => url).filter((u) => u.startsWith('/notes?'));
const lastParams = () => new URLSearchParams(calls().at(-1).split('?')[1]);
// The kind buttons carry their counts ("Notas" and then the number).
const kind = (label) => view.buttonMatching(new RegExp(`^${label}\\d+$`));
const wait = (ms) => act(async () => { await new Promise((r) => setTimeout(r, ms)); });

// What the server counts: each facet follows the other filters and ignores its own choice.
function facetsOf(p) {
  const base = (omit) => notes.filter((n) =>
    (omit === 'kind' || !p.get('kind') || n.kind === p.get('kind')) &&
    (omit === 'tag' || !p.get('tag') || n.tags.includes(p.get('tag'))) &&
    (!p.get('workId') || n.workId === Number(p.get('workId'))) &&
    (!p.get('q') || n.quote.includes(p.get('q'))));
  const kinds = { note: 0, highlight: 0, bookmark: 0 };
  base('kind').forEach((n) => { kinds[n.kind] += 1; });
  const tags = {};
  base('tag').forEach((n) => n.tags.forEach((t) => { tags[t] = (tags[t] ?? 0) + 1; }));
  return { kinds, tags: Object.entries(tags).sort((x, y) => y[1] - x[1] || x[0].localeCompare(y[0])).map(([tag, count]) => ({ tag, count })) };
}

async function open(all = [note(1)]) {
  notes = all;
  api.get.mockImplementation(async (url) => {
    if (url.startsWith('/notes/facets?')) return { data: facetsOf(new URLSearchParams(url.split('?')[1])) };
    if (!url.startsWith('/notes?')) throw new Error(`unexpected GET ${url}`);
    const p = new URLSearchParams(url.split('?')[1]);
    let found = notes;
    if (p.get('kind')) found = found.filter((n) => n.kind === p.get('kind'));
    if (p.get('workId')) found = found.filter((n) => n.workId === Number(p.get('workId')));
    if (p.get('tag')) found = found.filter((n) => n.tags.includes(p.get('tag')));
    if (p.get('q')) found = found.filter((n) => n.quote.includes(p.get('q')));
    const offset = Number(p.get('offset')), limit = Number(p.get('limit'));
    return { data: { data: found.slice(offset, offset + limit), total: found.length } };
  });
  api.delete.mockImplementation(async (url) => { notes = notes.filter((n) => `/notes/${n.id}` !== url); return {}; });
  view = await mount(<NotesPage />);
  await flush();
}
const type = async (value) => {
  await act(async () => {
    const input = document.body.querySelector('input[type="search"]');
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
};
const items = () => [...document.body.querySelectorAll('li[aria-label]')];

beforeEach(() => {
  vi.clearAllMocks();
  openBook = vi.fn();
  useGlobalStore.setState({ openBook });
});
afterEach(() => view.unmount());

describe('NotesPage: every note of the person (#13)', () => {
  it('lists the notes with the work each is from, and counts them', async () => {
    await open([note(1), note(2, { workId: 8, workTitle: 'Fundação', workAuthor: 'Isaac Asimov', kind: 'highlight' })]);
    expect(view.text()).toContain('2 registros privados');
    expect(items()).toHaveLength(2);
    expect(items()[0].textContent).toContain('Duna · Frank Herbert');
    expect(items()[1].textContent).toContain('Fundação · Isaac Asimov');
    expect(calls()).toEqual(['/notes?limit=20&offset=0']);
  });

  it('says one record in the singular and says what to do when there are none', async () => {
    await open([note(1)]);
    expect(view.text()).toContain('1 registro privado');
    view.unmount();
    await open([]);
    expect(view.text()).toContain('Nenhuma anotação ainda.');
    expect(view.text()).toContain('Só você os vê');
    expect(view.button('Exportar…').disabled).toBe(true);
  });

  it('keeps the text of a note whose work left the library, and marks the source unavailable', async () => {
    await open([note(1, { workId: null, workTitle: 'Obra retirada', sourceAvailable: false, fileId: null, fileAvailable: false, quote: 'Fica o texto' })]);
    const li = items()[0];
    expect(li.textContent).toContain('Fica o texto');
    expect(li.textContent).toContain('Obra retirada');
    expect(li.textContent).toContain('fonte indisponível');
    expect([...li.querySelectorAll('button')].some((b) => b.textContent === 'Obra retirada')).toBe(false); // no work to narrow to
    expect(view.text()).not.toContain('Abrir neste ponto');
  });

  it('searches after the person stops typing, with the text trimmed, and counts what was found', async () => {
    await open([note(1, { quote: 'areia' }), note(2, { quote: 'sal' })]);
    await type('  areia ');
    await wait(120);
    expect(calls()).toHaveLength(1); // not yet: still typing
    await wait(250);
    await flush();
    expect(lastParams().get('q')).toBe('areia');
    expect(view.text()).toContain('1 registro privado com estes filtros');
    expect(items()).toHaveLength(1);
  });

  it('narrows by kind, and Todas takes it away', async () => {
    await open([note(1), note(2, { kind: 'highlight' }), note(3, { kind: 'bookmark' })]);
    await view.click(kind('Destaques'));
    expect(lastParams().get('kind')).toBe('highlight');
    expect(kind('Destaques').getAttribute('aria-pressed')).toBe('true');
    expect(kind('Todas').getAttribute('aria-pressed')).toBe('false');
    expect(items()).toHaveLength(1);
    await view.click(kind('Todas'));
    expect(lastParams().has('kind')).toBe(false);
    expect(items()).toHaveLength(3);
  });

  it('narrows to a work by its title, with a filter that can be taken away', async () => {
    await open([note(1), note(2, { workId: 8, workTitle: 'Fundação' })]);
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === 'Duna'));
    expect(lastParams().get('workId')).toBe('7');
    expect(view.text()).toContain('Obra: Duna ✕');
    expect(view.text()).toContain('com estes filtros');
    expect(items()).toHaveLength(1);
    await view.click(view.buttonMatching(/^Obra: Duna/));
    expect(lastParams().has('workId')).toBe(false);
    expect(items()).toHaveLength(2);
  });

  it('narrows to a tag by clicking it, with a filter that can be taken away', async () => {
    await open([note(1, { tags: ['medo', 'ideia'] }), note(2, { tags: ['ideia'] }), note(3)]);
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === '#medo'));
    expect(lastParams().get('tag')).toBe('medo');
    expect(view.text()).toContain('Tag: #medo ✕');
    expect(view.text()).toContain('com estes filtros');
    expect(items()).toHaveLength(1);
    await view.click(view.buttonMatching(/^Tag: #medo/));
    expect(items()).toHaveLength(3);
  });

  it('clears every filter at once, and offers to only when one is set', async () => {
    await open([note(1, { tags: ['a'], quote: 'areia' })]);
    expect(view.button('Limpar filtros')).toBeUndefined();
    await view.click(kind('Notas'));
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === '#a'));
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === 'Duna'));
    await type('areia');
    await wait(350);
    await flush();
    expect(calls().at(-1)).toContain('q=areia');
    await view.click(view.button('Limpar filtros'));
    expect(calls().at(-1)).toBe('/notes?limit=20&offset=0');
    expect(document.body.querySelector('input[type="search"]').value).toBe('');
    expect(view.button('Limpar filtros')).toBeUndefined();
    expect(view.text()).not.toContain('com estes filtros');
  });

  it('says nothing matched when filters leave none, and not that there are no notes', async () => {
    await open([note(1)]);
    await view.click(kind('Marcadores'));
    expect(view.text()).toContain('Nenhuma anotação com estes filtros.');
    expect(view.text()).not.toContain('Nenhuma anotação ainda');
  });

  it('pages through the notes twenty at a time, newest first as the server sends them', async () => {
    await open(Array.from({ length: 45 }, (_, i) => note(i + 1)));
    expect(view.text()).toContain('1–20 de 45');
    expect(view.button('Anterior').disabled).toBe(true);
    await view.click(view.button('Próxima'));
    expect(calls().at(-1)).toBe('/notes?limit=20&offset=20');
    expect(view.text()).toContain('21–40 de 45');
    await view.click(view.button('Próxima'));
    expect(view.text()).toContain('41–45 de 45');
    expect(view.button('Próxima').disabled).toBe(true);
    expect(items()).toHaveLength(5);
    await view.click(view.button('Anterior'));
    expect(view.text()).toContain('21–40 de 45');
  });

  it('has no pages for twenty notes or fewer', async () => {
    await open(Array.from({ length: 20 }, (_, i) => note(i + 1)));
    expect(view.button('Próxima')).toBeUndefined();
  });

  it('goes back to the first page when a filter changes', async () => {
    await open(Array.from({ length: 45 }, (_, i) => note(i + 1)));
    await view.click(view.button('Próxima'));
    await view.click(kind('Notas'));
    expect(calls().at(-1)).toBe('/notes?kind=note&limit=20&offset=0');
  });

  it('goes back to the first page when a work or a tag is chosen from a later page', async () => {
    await open(Array.from({ length: 45 }, (_, i) => note(i + 1, { tags: ['medo'] })));
    await view.click(view.button('Próxima'));
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === 'Duna'));
    expect(calls().at(-1)).toBe('/notes?workId=7&limit=20&offset=0');
    await view.click(view.button('Próxima'));
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === '#medo'));
    expect(calls().at(-1)).toBe('/notes?tag=medo&workId=7&limit=20&offset=0');
  });

  it('goes back a page when the last note of the last page is deleted', async () => {
    await open(Array.from({ length: 21 }, (_, i) => note(i + 1)));
    await view.click(view.button('Próxima'));
    expect(items()).toHaveLength(1);
    await view.click(view.button('Excluir'));
    await view.click(view.button('Confirmar exclusão'));
    await flush();
    await flush();
    expect(api.delete).toHaveBeenCalledWith('/notes/21');
    expect(calls().at(-1)).toBe('/notes?limit=20&offset=0');
    expect(items()).toHaveLength(20);
    expect(view.text()).toContain('20 registros privados');
  });

  it('reopens the file at the place of the note', async () => {
    await open([note(1, { fileId: 70, workId: 7, locator: { type: 'pdf', page: 11 } })]);
    await view.click(view.button('Abrir neste ponto'));
    expect(openBook).toHaveBeenCalledWith(7, 70, { locator: { type: 'pdf', page: 11 }, context: { kind: 'note', quote: 'Trecho 1' } });
  });

  it('exports what the filters leave, after review', async () => {
    await open([note(1, { kind: 'highlight' })]);
    await view.click(kind('Destaques'));
    await view.click(view.button('Exportar…'));
    expect(view.dialog().textContent).toContain('1 anotação: só destaques.');
    await view.click(view.button('Cancelar'));
    expect(view.dialog()).toBeNull();
  });

  it('exports with the work and the tag that narrow the list', async () => {
    await open([note(1, { tags: ['medo'] })]);
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === 'Duna'));
    await view.click([...document.body.querySelectorAll('li button')].find((b) => b.textContent === '#medo'));
    await view.click(view.button('Exportar…'));
    expect(view.dialog().textContent).toContain('1 anotação: obra “Duna”, tag #medo.');
  });

  it('says it could not load the notes', async () => {
    api.get.mockRejectedValue(new Error('offline'));
    view = await mount(<NotesPage />);
    await flush();
    expect(view.text()).toContain('Não foi possível carregar as anotações.');
    expect(view.text()).not.toContain('Nenhuma anotação ainda');
  });
});

describe('NotesPage, the panel that narrows the list', () => {
  const sample = () => [
    note(1, { kind: 'note', tags: ['filosofia', 'poder'], quote: 'a' }),
    note(2, { kind: 'note', tags: ['filosofia'], quote: 'b' }),
    note(3, { kind: 'highlight', tags: ['ecologia'], quote: 'c' }),
    note(4, { kind: 'highlight', tags: ['ecologia', 'filosofia'], quote: 'd', workId: 8, workTitle: 'Fundação' }),
    note(5, { kind: 'bookmark', tags: [], quote: 'e' }),
  ];
  const facetCalls = () => api.get.mock.calls.map(([url]) => url).filter((u) => u.startsWith('/notes/facets?'));
  const tagButton = (name) => view.buttonMatching(new RegExp(`^#${name}\\d+$`));

  it('counts the kinds and the tags of what is stored', async () => {
    await open(sample());
    expect(kind('Todas').textContent).toBe('Todas5');
    expect(kind('Notas').textContent).toBe('Notas2');
    expect(kind('Destaques').textContent).toBe('Destaques2');
    expect(kind('Marcadores').textContent).toBe('Marcadores1');
    expect(tagButton('filosofia').textContent).toBe('#filosofia3');
    expect(tagButton('ecologia').textContent).toBe('#ecologia2');
    expect(tagButton('poder').textContent).toBe('#poder1');
  });

  it('narrows the list by a tag from the panel, and the kinds follow while the tags do not', async () => {
    await open(sample());
    await view.click(tagButton('ecologia'));
    await flush();
    expect(lastParams().get('tag')).toBe('ecologia');
    expect(tagButton('ecologia').getAttribute('aria-pressed')).toBe('true');
    // The kinds count under the tag; the tags still show what the others would give.
    expect(kind('Destaques').textContent).toBe('Destaques2');
    expect(kind('Notas').textContent).toBe('Notas0');
    expect(tagButton('poder')).toBeDefined();
    expect(facetCalls().at(-1)).toContain('tag=ecologia');
    // A click on it again takes it away.
    await view.click(tagButton('ecologia'));
    await flush();
    expect(lastParams().get('tag')).toBeNull();
    expect(kind('Notas').textContent).toBe('Notas2');
  });

  it('narrows by kind from the panel, and the tags follow the kind', async () => {
    await open(sample());
    await view.click(kind('Destaques'));
    await flush();
    expect(lastParams().get('kind')).toBe('highlight');
    expect(tagButton('ecologia').textContent).toBe('#ecologia2');
    expect(tagButton('poder')).toBeUndefined();
    expect(kind('Notas').textContent).toBe('Notas2');
    await view.click(kind('Destaques'));
    await flush();
    expect(lastParams().get('kind')).toBeNull();
  });

  it('asks for the counts under the same text and work as the list', async () => {
    await open(sample());
    await type('c');
    await wait(400);
    await flush();
    expect(facetCalls().at(-1)).toContain('q=c');
    expect(kind('Destaques').textContent).toBe('Destaques1');
    await type('');
    await wait(400);
    await flush();
    await view.click(view.button('Fundação'));
    await flush();
    expect(facetCalls().at(-1)).toContain('workId=8');
    // Under that work there is one note, a highlight with two tags.
    expect(kind('Todas').textContent).toBe('Todas1');
    expect(tagButton('ecologia').textContent).toBe('#ecologia1');
    expect(tagButton('poder')).toBeUndefined();
  });

  it('has no tags in the panel when the notes have none', async () => {
    await open([note(1, { tags: [] })]);
    expect(document.querySelector('[aria-label="Filtrar por tag"]')).toBeNull();
    expect(kind('Notas').textContent).toBe('Notas1');
  });

  it('reads the counts again when a note is deleted', async () => {
    await open(sample());
    expect(kind('Todas').textContent).toBe('Todas5');
    await view.click(view.button('Excluir'));
    await view.click(view.button('Confirmar exclusão'));
    await flush();
    expect(kind('Todas').textContent).toBe('Todas4');
    expect(tagButton('filosofia').textContent).toBe('#filosofia2');
  });
});
