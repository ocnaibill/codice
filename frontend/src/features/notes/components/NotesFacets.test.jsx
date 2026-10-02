import { describe, it, expect, afterEach, vi } from 'vitest';
import { mount } from '../../admin/testUtils';
import { NotesFacets, TAGS_SHOWN } from './NotesFacets';

let view;
afterEach(() => view?.unmount());

const tags = (n) => Array.from({ length: n }, (_, i) => ({ tag: `tag${String(i).padStart(2, '0')}`, count: n - i }));
const facets = (over = {}) => ({ kinds: { note: 4, highlight: 2, bookmark: 1 }, tags: tags(3), ...over });
const render = (props = {}) =>
  mount(<NotesFacets facets={facets()} kind="" onKind={vi.fn()} tag="" onTag={vi.fn()} {...props} />);
const named = (re) => view.buttonMatching(re);

describe('NotesFacets, the kinds', () => {
  it('says how many notes each kind has, and Todas is their sum', async () => {
    view = await render();
    expect(named(/^Todas7$/)).toBeDefined();
    expect(named(/^Notas4$/)).toBeDefined();
    expect(named(/^Destaques2$/)).toBeDefined();
    expect(named(/^Marcadores1$/)).toBeDefined();
  });

  it('shows 0 for a kind with no notes, and nothing while the counts are not there yet', async () => {
    view = await render({ facets: facets({ kinds: { note: 3 } }) });
    expect(named(/^Destaques0$/)).toBeDefined();
    expect(named(/^Todas3$/)).toBeDefined();
    view.unmount();
    view = await render({ facets: undefined });
    expect(named(/^Todas0$/)).toBeDefined();
    expect(view.text()).not.toContain('Tags');
  });

  it('marks the kind that is on, and a click on it takes it away; a click on another picks that one', async () => {
    const onKind = vi.fn();
    view = await render({ kind: 'highlight', onKind });
    expect(named(/^Destaques/).getAttribute('aria-pressed')).toBe('true');
    expect(named(/^Notas/).getAttribute('aria-pressed')).toBe('false');
    expect(named(/^Todas/).getAttribute('aria-pressed')).toBe('false');
    await view.click(named(/^Destaques/));
    expect(onKind).toHaveBeenLastCalledWith('');
    await view.click(named(/^Notas/));
    expect(onKind).toHaveBeenLastCalledWith('note');
    await view.click(named(/^Todas/));
    expect(onKind).toHaveBeenLastCalledWith('');
  });

  it('has Todas on when no kind is chosen', async () => {
    view = await render({ kind: '' });
    expect(named(/^Todas/).getAttribute('aria-pressed')).toBe('true');
  });
});

describe('NotesFacets, the tags', () => {
  it('lists the tags in the order given, each with its count', async () => {
    view = await render();
    const rows = [...document.querySelectorAll('[aria-label="Filtrar por tag"] button')].map((b) => b.textContent);
    expect(rows).toEqual(['#tag003', '#tag012', '#tag021']);
  });

  it('has no section of tags when there are none', async () => {
    view = await render({ facets: facets({ tags: [] }) });
    expect(document.querySelector('[aria-label="Filtrar por tag"]')).toBeNull();
    expect(view.text()).not.toContain('Tags');
  });

  it('marks the tag that is on whatever its case, and a click on it takes it away', async () => {
    const onTag = vi.fn();
    view = await render({ tag: 'TAG01', onTag });
    expect(named(/^#tag01/).getAttribute('aria-pressed')).toBe('true');
    expect(named(/^#tag00/).getAttribute('aria-pressed')).toBe('false');
    await view.click(named(/^#tag01/));
    expect(onTag).toHaveBeenLastCalledWith('');
    await view.click(named(/^#tag00/));
    expect(onTag).toHaveBeenLastCalledWith('tag00');
  });

  it('shows the commonest few and the rest on request, and back', async () => {
    view = await render({ facets: facets({ tags: tags(TAGS_SHOWN + 5) }) });
    const count = () => document.querySelectorAll('[aria-label="Filtrar por tag"] button').length;
    expect(count()).toBe(TAGS_SHOWN);
    expect(view.text()).toContain(`Tags ${TAGS_SHOWN + 5}`);
    await view.click(view.button(`Mostrar todas as ${TAGS_SHOWN + 5}`));
    expect(count()).toBe(TAGS_SHOWN + 5);
    await view.click(view.button('Mostrar menos'));
    expect(count()).toBe(TAGS_SHOWN);
  });

  it('offers no "mostrar todas" when they all fit', async () => {
    view = await render({ facets: facets({ tags: tags(TAGS_SHOWN) }) });
    expect(view.buttonMatching(/Mostrar/)).toBeUndefined();
  });

  it('keeps the tag that is on in view even when it is not among the first', async () => {
    view = await render({ facets: facets({ tags: tags(TAGS_SHOWN + 5) }), tag: `tag${String(TAGS_SHOWN + 3).padStart(2, '0')}` });
    const rows = [...document.querySelectorAll('[aria-label="Filtrar por tag"] button')];
    expect(rows).toHaveLength(TAGS_SHOWN + 1);
    expect(rows[0].textContent.startsWith(`#tag${TAGS_SHOWN + 3}`)).toBe(true);
    expect(rows[0].getAttribute('aria-pressed')).toBe('true');
  });

  it('does not repeat a tag that is on and already among the first', async () => {
    view = await render({ facets: facets({ tags: tags(TAGS_SHOWN + 5) }), tag: 'TAG02' });
    expect(document.querySelectorAll('[aria-label="Filtrar por tag"] button')).toHaveLength(TAGS_SHOWN);
  });
});
