import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { patch: vi.fn(), delete: vi.fn() } }));

import { mount } from '../../admin/testUtils';
import { NoteItem } from './NoteItem';

const note = {
  id: 1, kind: 'highlight', workId: 7, workTitle: 'Duna', workAuthor: 'Frank Herbert', fileId: 70, fileAvailable: true, sourceAvailable: true,
  quote: 'Fear is the mind-killer.', body: '', tags: ['medo', 'ideia'], locator: { type: 'pdf', page: 11 }, createdAt: '2026-09-19T10:00:00Z',
};
let view;
const render = (props = {}) => mount(<ul><NoteItem note={note} onOpenAt={vi.fn()} {...props} /></ul>);
beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('NoteItem in a list of every note', () => {
  it('says nothing of the work by default, as in the panel of a book', async () => {
    view = await render();
    expect(view.text()).not.toContain('Duna');
    expect(view.text()).not.toContain('Frank Herbert');
  });

  it('names the work and its author, and the work narrows the list when it can', async () => {
    const onFilterWork = vi.fn();
    view = await render({ showSource: true, onFilterWork });
    expect(view.text()).toContain('Duna · Frank Herbert');
    await view.click(view.button('Duna'));
    expect(onFilterWork).toHaveBeenCalledWith(note);
  });

  it('names the work as plain text when nothing is listening, and has no author line when there is none', async () => {
    view = await render({ showSource: true, note: { ...note, workAuthor: '' } });
    expect(view.button('Duna')).toBeUndefined();
    expect(view.text()).toContain('Duna');
    expect(view.text()).not.toContain('·');
  });

  it('marks the source unavailable, and leaves the title as text, when the work left the library', async () => {
    view = await render({ showSource: true, onFilterWork: vi.fn(), note: { ...note, sourceAvailable: false, workId: null } });
    expect(view.text()).toContain('fonte indisponível');
    expect(view.button('Duna')).toBeUndefined();
    expect(view.text()).toContain('Fear is the mind-killer.');
    view.unmount();
    view = await render({ showSource: true, onFilterWork: vi.fn() });
    expect(view.text()).not.toContain('fonte indisponível');
  });

  it('turns the tags into buttons that narrow the list, and leaves them as text otherwise', async () => {
    const onFilterTag = vi.fn();
    view = await render({ onFilterTag });
    await view.click(view.button('#ideia'));
    expect(onFilterTag).toHaveBeenCalledWith('ideia');
    view.unmount();
    view = await render();
    expect(view.button('#ideia')).toBeUndefined();
    expect(view.text()).toContain('#ideia');
  });
});

describe('NoteItem, the text of a note', () => {
  it('marks the concepts that its [[links]] point to, and the ones that do not exist yet', async () => {
    view = await render({
      note: { ...note, body: 'Ver [[Medo]] e [[Coragem]]', links: { Medo: { conceptId: 3, name: 'Medo', description: '' }, Coragem: null } },
    });
    expect(document.querySelector('[data-concept="3"]').textContent).toBe('Medo');
    expect(view.button('Coragem')).toBeDefined();
    expect(view.text()).toContain('Ver Medo e Coragem');
  });

  it('tells, in the field, that [[Concept]] links to a concept', async () => {
    view = await render();
    await view.click(view.button('Editar'));
    expect(document.querySelector('textarea[aria-label="Sua anotação"]').placeholder).toContain('[[Conceito]]');
  });
});
