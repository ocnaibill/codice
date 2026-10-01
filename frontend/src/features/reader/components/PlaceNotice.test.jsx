import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount } from '../../admin/testUtils';
import { PlaceNotice } from './PlaceNotice';

let view;
let onStay;
let onFromStart;
const show = (notice) => {
  onStay = vi.fn();
  onFromStart = vi.fn();
  return mount(<PlaceNotice notice={notice} onStay={onStay} onFromStart={onFromStart} />);
};
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

const note = { kind: 'note', label: 'PDF, página 12', reason: 'A página 12 não existe: o arquivo tem 10 páginas.', quote: 'Fear is the mind-killer.' };

describe('PlaceNotice (RF-014)', () => {
  it('says where the place pointed, why it cannot be opened, the passage kept, and that the note is safe', async () => {
    view = await show(note);
    const text = view.text();
    expect(text).toContain('Não foi possível abrir o ponto da anotação');
    expect(text).toContain('Ele apontava para PDF, página 12.');
    expect(text).toContain('A página 12 não existe: o arquivo tem 10 páginas.');
    expect(text).toContain('Fear is the mind-killer.');
    expect(text).toContain('A anotação continua guardada');
    expect(document.body.querySelector('[role="alert"]')).toBeTruthy();
  });

  it('lets the person stay where they are or open from the start', async () => {
    view = await show(note);
    await view.click(view.button('Ficar onde estou'));
    expect(onStay).toHaveBeenCalledTimes(1);
    expect(onFromStart).not.toHaveBeenCalled();
    await view.click(view.button('Abrir do começo'));
    expect(onFromStart).toHaveBeenCalledTimes(1);
  });

  it('names what asked for the place', async () => {
    for (const [kind, title] of [['search', 'o trecho encontrado'], ['equivalent', 'a posição equivalente'], ['nonsense', 'o ponto da anotação']]) {
      view = await show({ ...note, kind });
      expect(view.text()).toContain(`Não foi possível abrir ${title}`);
      view.unmount();
    }
    view = await show(note);
  });

  it('says the note is safe only for a note', async () => {
    view = await show({ ...note, kind: 'search' });
    expect(view.text()).not.toContain('A anotação continua guardada');
  });

  it('shows no quote line when there is none, and cuts a long one', async () => {
    view = await show({ ...note, quote: '' });
    expect(document.body.querySelector('blockquote')).toBeNull();
    view.unmount();
    view = await show({ ...note, quote: 'x'.repeat(400) });
    expect(document.body.querySelector('blockquote').textContent).toBe(`${'x'.repeat(280)}…`);
    view.unmount();
    view = await show({ ...note, quote: 'y'.repeat(280) });
    expect(document.body.querySelector('blockquote').textContent).toBe('y'.repeat(280));
  });

  it('leaves the label out when there is none, and adds what else was found', async () => {
    view = await show({ ...note, label: null, also: 'A posição salva também falhou.' });
    expect(view.text()).not.toContain('Ele apontava para');
    expect(view.text()).toContain('A posição salva também falhou.');
  });

  it('for the saved position there is only one thing to do: understand, because the start is already open', async () => {
    view = await show({ kind: 'saved', label: 'PDF, página 40', reason: 'A página 40 não existe. Abrimos do começo.' });
    expect(view.text()).toContain('Sua posição salva não existe mais neste arquivo');
    expect(view.button('Abrir do começo')).toBeUndefined();
    expect(view.button('Ficar onde estou')).toBeUndefined();
    await view.click(view.button('Entendi'));
    expect(onStay).toHaveBeenCalledTimes(1);
  });
});
