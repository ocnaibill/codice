import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { PersonNames } from './PersonNames';

const herbert = { id: 1, name: 'Frank Herbert', works: 3, titles: ['Duna', 'Messias de Duna', 'Filhos de Duna'], suggestion: { family: 'Herbert', given: 'Frank' } };
const marquez = { id: 2, name: 'Gabriel García Márquez', works: 1, titles: ['Cólera'], suggestion: { family: 'Márquez', given: 'Gabriel García' } };
const duo = { id: 3, name: 'Alan Moore & Dave Gibbons', works: 1, titles: ['Watchmen'], suggestion: null };
let view;
let answers;

const page = (data, extra = {}) => ({ data: { data, total: data.length, page: 1, limit: 20, totalPages: 1, ...extra } });

async function open(pending = [herbert, marquez, duo], dealt = []) {
  answers = { pending: page(pending), done: page(dealt) };
  api.get.mockImplementation(async (url, config) => {
    if (url !== '/admin/people/names') throw new Error(`unexpected GET ${url}`);
    return (config?.params?.state === 'done' ? answers.done : answers.pending);
  });
  api.put.mockResolvedValue({});
  view = await mount(<PersonNames />);
}
const namesCalls = () => api.get.mock.calls.filter(([url]) => url === '/admin/people/names').map(([, config]) => config.params);
const chip = (word, within = document.body) => [...within.querySelectorAll('[aria-pressed]')].find((b) => b.textContent === word && b.closest('[role="group"]')?.getAttribute('aria-label')?.startsWith('Palavras'));
const row = (name) => document.body.querySelector(`[role="group"][aria-label="Palavras do sobrenome de ${name}"]`).closest('li');
const inDialog = (label) => [...view.dialog().querySelectorAll('button')].find((b) => b.textContent.trim() === label);
const clickIn = (li, label) => view.click([...li.querySelectorAll('button')].find((b) => b.textContent.trim() === label));
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('PersonNames: the names to divide', () => {
  it('shows each person with the works they have and starts from the surname proposed', async () => {
    await open();
    expect(view.text()).toContain('3 nomes a dividir.');
    expect(view.text()).toContain('Frank Herbert');
    expect(view.text()).toContain('3 obras: Duna, Messias de Duna, Filhos de Duna');
    expect(view.text()).toContain('1 obra: Cólera');
    expect(chip('Herbert').getAttribute('aria-pressed')).toBe('true');
    expect(chip('Frank').getAttribute('aria-pressed')).toBe('false');
    expect(row('Frank Herbert').textContent).toContain('Sobrenome primeiro: Herbert, Frank');
    expect(namesCalls()).toEqual([{ page: 1 }]);
  });

  it('has no way to take a person back to the list when they are still in it', async () => {
    await open();
    expect(view.button('Voltar para a lista')).toBeUndefined();
    expect(view.button('Salvar')).toBeUndefined();
  });

  it('keeps the buttons of the page still while something is being saved', async () => {
    await open();
    api.put.mockImplementationOnce(() => new Promise(() => {}));
    await clickIn(row('Frank Herbert'), 'Confirmar');
    for (const label of ['Confirmar', 'Sem sobrenome']) {
      for (const li of document.body.querySelectorAll('li')) {
        expect([...li.querySelectorAll('button')].find((b) => b.textContent === label).disabled).toBe(true);
      }
    }
  });

  it('says there are more works than the titles shown', async () => {
    await open([{ ...herbert, works: 5 }]);
    expect(view.text()).toContain('5 obras: Duna, Messias de Duna, Filhos de Duna…');
  });

  it('proposes nothing for a line that names several people: the person picks the words', async () => {
    await open();
    const li = row('Alan Moore & Dave Gibbons');
    expect([...li.querySelectorAll('[aria-pressed="true"]')]).toHaveLength(0);
    expect(li.textContent).toContain('Toque nas palavras do sobrenome.');
    expect([...li.querySelectorAll('button')].find((b) => b.textContent === 'Confirmar').disabled).toBe(true);
  });

  it('lets the person change the words of the surname, for a compound one', async () => {
    await open();
    const li = row('Gabriel García Márquez');
    expect(li.textContent).toContain('Sobrenome primeiro: Márquez, Gabriel García');
    await view.click(chip('García', li));
    expect(chip('García', li).getAttribute('aria-pressed')).toBe('true');
    expect(li.textContent).toContain('Sobrenome primeiro: García Márquez, Gabriel');
    await view.click(chip('García', li));
    expect(li.textContent).toContain('Sobrenome primeiro: Márquez, Gabriel García');
    await view.click(chip('Márquez', li));
    expect(li.textContent).toContain('Toque nas palavras do sobrenome.');
    expect([...li.querySelectorAll('button')].find((b) => b.textContent === 'Confirmar').disabled).toBe(true);
  });

  it('tells apart two equal words of a name by their place', async () => {
    await open([{ id: 7, name: 'Ana Ana Silva', works: 1, titles: ['x'], suggestion: { family: 'Silva', given: 'Ana Ana' } }]);
    const li = row('Ana Ana Silva');
    const anas = [...li.querySelectorAll('[aria-pressed]')].filter((b) => b.textContent === 'Ana');
    expect(anas).toHaveLength(2);
    await view.click(anas[1]);
    expect(li.textContent).toContain('Sobrenome primeiro: Ana Silva, Ana');
    expect(anas[0].getAttribute('aria-pressed')).toBe('false');
    expect(anas[1].getAttribute('aria-pressed')).toBe('true');
  });

  it('confirms the surname with the words chosen, and says how the name reads', async () => {
    await open();
    const li = row('Gabriel García Márquez');
    await view.click(chip('García', li));
    await clickIn(li, 'Confirmar');
    expect(api.put).toHaveBeenCalledTimes(1);
    expect(api.put).toHaveBeenCalledWith('/admin/people/2/name', { family: 'García Márquez', given: 'Gabriel' });
    expect(view.text()).toContain('“Gabriel García Márquez” aparece como “García Márquez, Gabriel” na ordem sobrenome primeiro.');
  });

  it('asks the lists again after saving, since the person is no longer to divide', async () => {
    await open();
    const before = namesCalls().length;
    answers.pending = page([marquez, duo]);
    await clickIn(row('Frank Herbert'), 'Confirmar');
    expect(namesCalls().length).toBeGreaterThan(before);
    expect(document.body.querySelector('[role="group"][aria-label="Palavras do sobrenome de Frank Herbert"]')).toBeNull();
    expect(view.text()).toContain('2 nomes a dividir.');
  });

  it('says that a name has no surname, and that it then reads as it is written', async () => {
    await open();
    await clickIn(row('Alan Moore & Dave Gibbons'), 'Sem sobrenome');
    expect(api.put).toHaveBeenCalledWith('/admin/people/3/name', { undivided: true });
    expect(view.text()).toContain('“Alan Moore & Dave Gibbons” fica sem sobrenome: aparece sempre como está escrito.');
  });

  it('shows what went wrong when the server refuses', async () => {
    await open();
    api.put.mockRejectedValueOnce({ response: { status: 400, data: 'family and given must be made of the words of the name' } });
    await clickIn(row('Frank Herbert'), 'Confirmar');
    expect(view.container.querySelector('[role="alert"]')).not.toBeNull();
    expect(view.text()).not.toContain('aparece como');
  });

  it('shows the count in the singular, and that there is nothing left, with and without a search', async () => {
    await open([herbert]);
    expect(view.text()).toContain('1 nome a dividir.');
    view.unmount();
    await open([]);
    expect(view.text()).toContain('Nenhum nome a dividir.');
    expect(view.text()).not.toContain('com essa busca');
    await view.type(view.container.querySelector('input[aria-label="Buscar um nome"]'), 'zzz');
    await view.click(view.button('Buscar'));
    expect(view.text()).toContain('Nenhum nome a dividir com essa busca.');
  });

  it('shows that the names could not be loaded, and asks again', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    view = await mount(<PersonNames />);
    expect(view.text()).toContain('Não foi possível carregar os nomes.');
  });
});

describe('PersonNames: the proposals of a page, at once', () => {
  it('confirms the surname of every person who has one marked, in the order of the page, after asking', async () => {
    await open();
    expect(view.button('Confirmar as 2 sugestões desta página')).toBeTruthy();
    await view.click(chip('García', row('Gabriel García Márquez')));
    await view.click(view.button('Confirmar as 2 sugestões desta página'));
    expect(api.put).not.toHaveBeenCalled();
    expect(view.dialog().textContent).toContain('2 pessoas passam a ter o sobrenome que está marcado no nome dela');
    expect(view.dialog().textContent).toContain('dá para corrigir em “Já resolvidos”');
    await view.click(inDialog('Confirmar'));
    expect(api.put.mock.calls).toEqual([
      ['/admin/people/1/name', { family: 'Herbert', given: 'Frank' }],
      ['/admin/people/2/name', { family: 'García Márquez', given: 'Gabriel' }],
    ]);
    expect(view.text()).toContain('2 pessoas têm agora o sobrenome confirmado.');
  });

  it('leaves out the people who have no surname marked, and says it in the singular for one', async () => {
    await open([herbert, duo]);
    expect(view.button('Confirmar a sugestão desta página')).toBeTruthy();
    await view.click(view.button('Confirmar a sugestão desta página'));
    expect(view.dialog().textContent).toContain('1 pessoa passa a ter');
    await view.click(inDialog('Confirmar'));
    expect(api.put).toHaveBeenCalledTimes(1);
    expect(view.text()).toContain('1 pessoa tem agora o sobrenome confirmado.');
  });

  it('is not offered when no person has a surname marked, and does nothing when it is cancelled', async () => {
    await open([duo]);
    expect(view.buttonMatching(/desta página/)).toBeUndefined();
    view.unmount();
    await open();
    await view.click(view.button('Confirmar as 2 sugestões desta página'));
    await view.click(view.button('Cancelar'));
    expect(api.put).not.toHaveBeenCalled();
    expect(view.dialog()).toBeNull();
  });

  it('shows how far it got, and stops at the first one the server refuses', async () => {
    await open();
    let release;
    api.put
      .mockResolvedValueOnce({})
      .mockImplementationOnce(() => new Promise((resolve) => { release = resolve; }));
    await view.click(view.button('Confirmar as 2 sugestões desta página'));
    await view.click(inDialog('Confirmar'));
    expect(view.text()).toContain('Salvando… 1 de 2.');
    expect(view.buttonMatching(/desta página/).disabled).toBe(true);
    await act(async () => { release({}); });
    await flush();
    expect(view.text()).toContain('2 pessoas têm agora o sobrenome confirmado.');
    expect(view.text()).not.toContain('Salvando');

    api.put.mockReset();
    api.put.mockResolvedValueOnce({}).mockRejectedValueOnce({ response: { status: 400, data: 'family and given must be made of the words of the name' } });
    await view.click(view.button('Confirmar as 2 sugestões desta página'));
    await view.click(inDialog('Confirmar'));
    expect(api.put).toHaveBeenCalledTimes(2);
    expect(view.container.querySelector('[role="alert"]')).not.toBeNull();
    expect(view.text()).not.toContain('têm agora');
  });

  it('asks the lists again even when the server refuses one, since the ones before it were saved', async () => {
    await open();
    api.put.mockResolvedValueOnce({}).mockRejectedValueOnce({ response: { status: 400, data: 'family and given must be made of the words of the name' } });
    const before = namesCalls().length;
    await view.click(view.button('Confirmar as 2 sugestões desta página'));
    await view.click(inDialog('Confirmar'));
    expect(view.container.querySelector('[role="alert"]')).not.toBeNull();
    expect(namesCalls().length).toBeGreaterThan(before);
  });

  it('starts again from nothing: the sentence of the last time is not shown while it runs', async () => {
    await open([herbert]);
    await clickIn(row('Frank Herbert'), 'Confirmar');
    expect(view.text()).toContain('aparece como “Herbert, Frank”');
    api.put.mockImplementationOnce(() => new Promise(() => {}));
    await clickIn(row('Frank Herbert'), 'Confirmar');
    expect(view.text()).toContain('Salvando… 0 de 1.');
    expect(view.text()).not.toContain('aparece como');
  });
});

describe('PersonNames: the search and the pages', () => {
  it('searches by the name and goes back to the first page', async () => {
    await open();
    await view.type(view.container.querySelector('input[aria-label="Buscar um nome"]'), '  garcia ');
    await view.click(view.button('Buscar'));
    expect(namesCalls().at(-1)).toEqual({ q: 'garcia', page: 1 });
  });

  it('goes back to the first page when the search changes, and when the other list is chosen', async () => {
    await open();
    api.get.mockImplementation(async (url, config) => page([herbert], { total: 45, totalPages: 3, page: config.params.page }));
    await view.type(view.container.querySelector('input[aria-label="Buscar um nome"]'), 'a');
    await view.click(view.button('Buscar'));
    await view.click(view.button('Próxima'));
    expect(namesCalls().at(-1)).toEqual({ q: 'a', page: 2 });
    await view.type(view.container.querySelector('input[aria-label="Buscar um nome"]'), 'ab');
    await view.click(view.button('Buscar'));
    expect(namesCalls().at(-1)).toEqual({ q: 'ab', page: 1 });
    await view.click(view.button('Próxima'));
    expect(namesCalls().at(-1)).toEqual({ q: 'ab', page: 2 });
    await view.click(view.button('Já resolvidos'));
    expect(namesCalls().at(-1)).toEqual({ state: 'done', q: 'ab', page: 1 });
  });

  it('walks through the pages', async () => {
    await open();
    api.get.mockImplementation(async (url, config) => page([herbert], { total: 45, totalPages: 3, page: config.params.page }));
    await view.click(view.button('Buscar'));
    await view.type(view.container.querySelector('input[aria-label="Buscar um nome"]'), 'a');
    await view.click(view.button('Buscar'));
    expect(view.text()).toContain('Página 1 de 3');
    expect(view.button('Anterior').disabled).toBe(true);
    await view.click(view.button('Próxima'));
    expect(namesCalls().at(-1)).toEqual({ q: 'a', page: 2 });
    await view.click(view.button('Próxima'));
    expect(view.text()).toContain('Página 3 de 3');
    expect(view.button('Próxima').disabled).toBe(true);
    await view.click(view.button('Anterior'));
    expect(namesCalls().at(-1)).toEqual({ q: 'a', page: 2 });
  });

  it('goes back to the last page there is when the one it was on has nobody left', async () => {
    await open();
    api.get.mockImplementation(async (url, config) => page([herbert], { total: 45, totalPages: 3, page: config.params.page }));
    await view.type(view.container.querySelector('input[aria-label="Buscar um nome"]'), 'a');
    await view.click(view.button('Buscar'));
    await view.click(view.button('Próxima'));
    await view.click(view.button('Próxima'));
    expect(view.text()).toContain('Página 3 de 3');
    // The only person of the last page is dealt with: it is empty now, and the list has 2 pages.
    api.get.mockImplementation(async (url, config) => page(config.params.page === 3 ? [] : [herbert], { total: 21, totalPages: 2, page: config.params.page }));
    await clickIn(row('Frank Herbert'), 'Confirmar');
    expect(namesCalls().at(-1)).toEqual({ q: 'a', page: 2 });
    expect(view.text()).toContain('Página 2 de 2');
  });
});

describe('PersonNames: the ones already dealt with', () => {
  const gabo = { id: 2, name: 'Gabriel García Márquez', works: 1, titles: ['Cólera'], family: 'García Márquez', given: 'Gabriel' };
  const msf = { id: 5, name: 'Médicos Sem Fronteiras', works: 2, titles: ['Relatório'], undivided: true };

  async function openDealt() {
    await open([herbert], [gabo, msf]);
    await view.click(view.button('Já resolvidos'));
  }

  it('asks for the other list, and says how many there are', async () => {
    await openDealt();
    expect(namesCalls().at(-1)).toEqual({ state: 'done', page: 1 });
    expect(view.text()).toContain('2 pessoas resolvidas.');
    expect(view.button('Já resolvidos').getAttribute('aria-pressed')).toBe('true');
    expect(view.button('A dividir').getAttribute('aria-pressed')).toBe('false');
    view.unmount();
    await open([], [gabo]);
    await view.click(view.button('Já resolvidos'));
    expect(view.text()).toContain('1 pessoa resolvida.');
    view.unmount();
    await open([], []);
    await view.click(view.button('Já resolvidos'));
    expect(view.text()).toContain('Nenhum nome resolvido ainda.');
  });

  it('starts from the surname the person has, and saves only what changed', async () => {
    await openDealt();
    const li = row('Gabriel García Márquez');
    expect(chip('García', li).getAttribute('aria-pressed')).toBe('true');
    expect(chip('Márquez', li).getAttribute('aria-pressed')).toBe('true');
    expect(chip('Gabriel', li).getAttribute('aria-pressed')).toBe('false');
    const save = () => [...li.querySelectorAll('button')].find((b) => b.textContent === 'Salvar');
    expect(save().disabled).toBe(true);
    await view.click(chip('García', li));
    expect(save().disabled).toBe(false);
    await view.click(save());
    expect(api.put).toHaveBeenCalledWith('/admin/people/2/name', { family: 'Márquez', given: 'Gabriel García' });
  });

  it('shows a name with no surname as it is, with no way to say so again, and lets the person choose one', async () => {
    await openDealt();
    const li = row('Médicos Sem Fronteiras');
    expect(li.textContent).toContain('Sem sobrenome: aparece sempre como está escrito.');
    expect([...li.querySelectorAll('button')].some((b) => b.textContent === 'Sem sobrenome')).toBe(false);
    expect([...li.querySelectorAll('button')].find((b) => b.textContent === 'Salvar').disabled).toBe(true);
    await view.click(chip('Fronteiras', li));
    expect(li.textContent).toContain('Sobrenome primeiro: Fronteiras, Médicos Sem');
    await clickIn(li, 'Salvar');
    expect(api.put).toHaveBeenCalledWith('/admin/people/5/name', { family: 'Fronteiras', given: 'Médicos Sem' });
  });

  it('takes a person back to the list to divide', async () => {
    await openDealt();
    await clickIn(row('Gabriel García Márquez'), 'Voltar para a lista');
    expect(api.put).toHaveBeenCalledWith('/admin/people/2/name', { family: '', given: '' });
    expect(view.text()).toContain('“Gabriel García Márquez” voltou para a lista dos nomes a dividir.');
  });

  it('can say that a name already divided has no surname', async () => {
    await openDealt();
    await clickIn(row('Gabriel García Márquez'), 'Sem sobrenome');
    expect(api.put).toHaveBeenCalledWith('/admin/people/5'.replace('5', '2') + '/name', { undivided: true });
  });

  it('offers no "confirm the proposals" here, and starts the page and the message again when the list changes', async () => {
    await openDealt();
    expect(view.buttonMatching(/desta página/)).toBeUndefined();
    await clickIn(row('Médicos Sem Fronteiras'), 'Voltar para a lista');
    expect(view.text()).toContain('voltou para a lista');
    await view.click(view.button('A dividir'));
    expect(view.text()).not.toContain('voltou para a lista');
    expect(namesCalls().at(-1)).toEqual({ page: 1 });
  });
});
