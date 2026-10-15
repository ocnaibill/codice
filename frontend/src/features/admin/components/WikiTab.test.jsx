import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import axe from 'axe-core';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { act } from 'react';
import { WikiTab } from './WikiTab';

let view;
const ownerProvider = (over = {}) => ({
  configured: true, kind: 'deepseek', model: 'deepseek-chat', host: 'api.deepseek.com', remote: true,
  baseUrl: 'https://api.deepseek.com', keySet: true, keyHint: '…4f2a', policy: 'A DeepSeek recebe o texto dos capítulos.', masterKey: true, ...over,
});
const run = (over = {}) => ({
  id: 1, scope: { kind: 'series', id: 5 }, title: 'Scythe', origin: 'manual', state: 'running', stage: 'map',
  progress: { text: { done: 4, total: 4 }, map: { done: 23, total: 47 } }, place: 'A Nuvem, capítulo 12', errors: [], resumes: 0,
  pauseRequested: false, cancelRequested: false, heartbeatAt: new Date().toISOString(), createdAt: '2026-10-15T12:00:00Z',
  startedAt: new Date(Date.now() - 10 * 60000).toISOString(), finishedAt: null, ...over,
});
const item = (over = {}) => ({ kind: 'work', id: 9, title: 'Addie', books: 1, readable: 1, excluded: false, wiki: null, run: null, ...over });

const settings = (over = {}) => ({
  minChapters: 5, default: 5, max: 200, previewMin: 5,
  preview: [{ projectId: 1, title: 'Scythe', entities: 166, pages: 3, would: 63, byKind: { person: 34 } }], ...over,
});

async function open({ provider = ownerProvider(), runs = [], items = [item()], isOwner = true, cut = settings() } = {}) {
  api.get.mockImplementation(async (url, config) => {
    if (url === '/admin/wiki-provider') return { data: provider };
    if (url === '/admin/wiki-settings') return { data: { ...cut, previewMin: config?.params?.min ?? cut.minChapters } };
    if (url === '/admin/wiki-runs') return { data: { runs } };
    if (url === '/admin/wiki-library') return { data: { items: typeof items === 'function' ? items(config?.params ?? {}) : items, total: 1 } };
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockImplementation(async (url, body) => ({ data: url === '/admin/wiki-provider' ? ownerProvider({ model: body.model }) : url === '/admin/wiki-settings' ? settings({ minChapters: body.minChapters }) : {} }));
  api.post.mockImplementation(async (url) => {
    if (url === '/admin/wiki-provider/test') return { data: { ok: true, ms: 840, reply: 'OK' } };
    if (url === '/admin/wiki-runs/all') return { data: { queued: 3 } };
    return { data: { runId: 7 } };
  });
  api.delete.mockResolvedValue({ data: { configured: false } });
  view?.unmount();
  view = await mount(<WikiTab isOwner={isOwner} />);
  await flush();
}
const field = (label) => [...document.body.querySelectorAll('label')].find((l) => l.textContent.trim().startsWith(label))?.querySelector('input, select');
const setValue = async (el, value) => {
  await act(async () => {
    const proto = el.tagName === 'SELECT' ? window.HTMLSelectElement.prototype : window.HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, value);
    el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }));
  });
};

afterEach(() => {
  view?.unmount();
  view = null;
});
beforeEach(() => vi.clearAllMocks());

describe('WikiTab: the provider', () => {
  it('shows the owner what is in use, the data policy, and never the key', async () => {
    await open();
    expect(view.text()).toContain('Em uso: DeepSeek · deepseek-chat · api.deepseek.com');
    expect(view.text()).toContain('remoto: o texto sai do servidor');
    expect(view.text()).toContain('A DeepSeek recebe o texto dos capítulos.');
    const key = field('Chave de API');
    expect(key.type).toBe('password');
    expect(key.value).toBe('');
    expect(key.placeholder).toContain('Guardada (…4f2a)');
  });

  it('saves the model without the key when none is typed, and with it when one is', async () => {
    await open();
    await setValue(field('Modelo'), 'deepseek-reasoner');
    await view.click(view.button('Salvar'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/wiki-provider', { kind: 'deepseek', baseUrl: 'https://api.deepseek.com', model: 'deepseek-reasoner', key: '', clearKey: false });
    await view.type(field('Chave de API'), 'sk-nova');
    await view.click(view.button('Salvar'));
    expect(api.put.mock.calls.at(-1)[1].key).toBe('sk-nova');
    // The key typed is cleared once saved.
    expect(field('Chave de API').value).toBe('');
  });

  it('tests the model and says how it went', async () => {
    await open();
    await view.click(view.button('Testar'));
    expect(view.text()).toContain('O modelo respondeu em 0.8 s: “OK”');
    api.post.mockResolvedValueOnce({ data: { ok: false, problem: 'O servidor recusou a chave.' } });
    await view.click(view.button('Testar'));
    expect(view.text()).toContain('O servidor recusou a chave.');
  });

  it('turns the generation off only after confirming', async () => {
    await open();
    await view.click(view.button('Desligar'));
    expect(api.delete).not.toHaveBeenCalled();
    expect(view.dialog().textContent).toContain('as que estão na fila esperam');
    await view.click([...view.dialog().querySelectorAll('button')].find((b) => b.textContent === 'Desligar'));
    expect(api.delete).toHaveBeenCalledWith('/admin/wiki-provider');
  });

  it('says what is missing on the server: the master key, or a key it cannot open', async () => {
    await open({ provider: ownerProvider({ masterKey: false, keyUnreadable: true }) });
    expect(view.text()).toContain('defina CODICE_SECRET_KEY');
    expect(view.text()).toContain('Digite a chave do provedor de novo');
  });

  it('shows admins which provider is in use, with no form', async () => {
    await open({ isOwner: false, provider: { configured: true, kind: 'openai', model: 'm', host: 'tokenharbor.ai', remote: true } });
    expect(view.text()).toContain('Em uso: Compatível com OpenAI · m · tokenharbor.ai');
    expect(document.body.querySelector('form[aria-label="Provedor da wiki"]')).toBeNull();
    await open({ isOwner: false, provider: { configured: false, remote: false } });
    expect(view.text()).toContain('Só o dono do acervo pode configurar.');
    expect(view.button('Gerar').disabled).toBe(true);
    expect(view.text()).toContain('Para gerar, configure o provedor acima.');
  });
});

describe('WikiTab: the cut for full pages (DEC-186)', () => {
  it('shows the cut in use and what each wiki would have, and the owner changes it', async () => {
    await open();
    const input = field('Mínimo de capítulos');
    expect(input.value).toBe('5');
    expect(view.text()).toContain('Em uso: 5 (o padrão)');
    expect(view.text()).toContain('63 de 166 entidades teriam página com 5 capítulos ou mais');
    expect(view.text()).toContain('(hoje: 3)');
    const saves = () => [...document.body.querySelectorAll('button')].filter((b) => b.textContent === 'Salvar');
    expect(saves().at(-1).disabled).toBe(true);
    await view.type(input, '3');
    await flush();
    expect(api.get.mock.calls.filter(([url]) => url === '/admin/wiki-settings').at(-1)[1]).toEqual({ params: { min: 3 } });
    expect(saves().at(-1).disabled).toBe(false);
    await view.click(saves().at(-1));
    expect(api.put).toHaveBeenLastCalledWith('/admin/wiki-settings', { minChapters: 3 });
  });

  it('is only read by an admin', async () => {
    await open({ isOwner: false, provider: { configured: true, kind: 'openai', model: 'm', host: 'x.com', remote: true } });
    expect(field('Mínimo de capítulos').disabled).toBe(true);
    expect([...document.body.querySelectorAll('button')].some((b) => b.textContent === 'Salvar')).toBe(false);
  });

  it('says that there is nothing to count before the first wiki', async () => {
    await open({ cut: settings({ preview: [] }) });
    expect(view.text()).toContain('Nenhuma wiki gerada ainda');
  });
});

describe('WikiTab: the queue', () => {
  it('says the stage, the counts, where it is, the time and what went wrong', async () => {
    await open({ runs: [run({ errors: [{ at: 'x', message: 'Limite de pedidos (429), esperando 30 s' }], resumes: 1 }), run({ id: 2, title: 'Addie', state: 'queued', stage: '', progress: {}, position: 1, startedAt: null })] });
    const rows = [...document.body.querySelectorAll('li')].filter((li) => li.querySelector('[aria-label="Etapas"]') || li.textContent.includes('Na fila'));
    const first = rows[0].textContent;
    expect(first).toContain('Scythe');
    expect(first).toContain('Gerando');
    expect(first).toContain('em A Nuvem, capítulo 12');
    expect(first).toContain('10 min');
    expect(first).toContain('restantes');
    expect(first).toContain('retomada 1×');
    expect(first).toContain('Limite de pedidos (429)');
    expect(rows[0].querySelector('[aria-current="step"]').textContent).toContain('Mapear capítulos23 de 47');
    expect(view.text()).toContain('Na fila · 1º');
  });

  it('says what the run has asked of the provider, and how much the cache already had', async () => {
    await open({ runs: [run({ progress: { map: { done: 3, total: 9 }, usage: { requests: 1240, hits: 310, tokensIn: 1_500_000, tokensOut: 250_000, throttled: 2 } } })] });
    expect(view.text()).toContain('1.240 pedidos · 310 já guardados · 1,8 mi de tokens · 2 esperas por limite do servidor');
  });

  it('pauses, resumes and cancels', async () => {
    await open({ runs: [run(), run({ id: 2, title: 'Addie', state: 'paused', stage: '', progress: {} })] });
    await view.click(view.button('Pausar'));
    expect(api.post).toHaveBeenLastCalledWith('/admin/wiki-runs/1/pause');
    await view.click(view.button('Retomar'));
    expect(api.post).toHaveBeenLastCalledWith('/admin/wiki-runs/2/resume');
    await view.click(view.buttonMatching(/^Cancelar$/));
    expect(api.post).toHaveBeenLastCalledWith('/admin/wiki-runs/1/cancel');
  });

  it('warns when the running one has gone silent', async () => {
    await open({ runs: [run({ heartbeatAt: new Date(Date.now() - 10 * 60000).toISOString() })] });
    expect(view.text()).toContain('O worker não dá notícia há mais de cinco minutos');
  });
});

describe('WikiTab: the works and series', () => {
  it('generates one, marks an exception, and generates for all after confirming', async () => {
    await open({ items: [item(), item({ kind: 'series', id: 5, title: 'Scythe', books: 4, readable: 3, excluded: true, wiki: { projectId: 2, state: 'ready', review: 'pending' } })] });
    expect(view.text()).toContain('série · 4 livros');
    expect(view.text()).toContain('3 com texto para ler');
    expect(view.text()).toContain('A revisar');
    await view.click(view.button('Gerar'));
    expect(api.post).toHaveBeenLastCalledWith('/admin/wiki-runs', { kind: 'work', id: 9 });
    const boxes = [...document.body.querySelectorAll('label')].filter((l) => l.textContent.includes('Não gerar wiki')).map((l) => l.querySelector('input'));
    expect(boxes.map((b) => b.checked)).toEqual([false, true]);
    await view.click(boxes[1]);
    expect(api.put).toHaveBeenLastCalledWith('/admin/wiki-exclusions', { kind: 'series', id: 5, excluded: false });
    await view.click(view.button('Gerar para todas que ainda não têm wiki'));
    expect(api.post).not.toHaveBeenCalledWith('/admin/wiki-runs/all');
    await view.click([...view.dialog().querySelectorAll('button')].find((b) => b.textContent === 'Colocar na fila'));
    expect(api.post).toHaveBeenLastCalledWith('/admin/wiki-runs/all');
    expect(view.text()).toContain('3 na fila.');
  });

  it('asks the server for the filter chosen', async () => {
    await open();
    await view.click(view.button('Exceções'));
    expect(api.get.mock.calls.filter(([url]) => url === '/admin/wiki-library').at(-1)[1].params).toMatchObject({ filter: 'excluded', offset: 0 });
  });

  it('cannot generate one that already has a live generation', async () => {
    await open({ items: [item({ run: { id: 3, state: 'queued' } })] });
    expect(view.button('Gerar').disabled).toBe(true);
    expect(view.text()).toContain('Na fila');
  });

  it('has no accessibility violation (axe)', async () => {
    await open({ runs: [run()], items: [item()] });
    const result = await axe.run(document.body, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
      rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
    });
    expect(result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`)).toEqual([]);
  });
});
