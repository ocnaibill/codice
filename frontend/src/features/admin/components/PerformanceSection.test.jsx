import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { PerformanceSection } from './PerformanceSection';

let view;
let server;
const BASE = {
  values: { catalogReads: 8, dedupeJobs: 1, ocrPages: 1, ocrThreads: 1 },
  defaults: { catalogReads: 8, dedupeJobs: 1, ocrPages: 1, ocrThreads: 1 },
  overridden: [],
  limits: { catalogReads: { min: 1, max: 20 }, dedupeJobs: { min: 1, max: 4 }, ocrPages: { min: 1, max: 8 }, ocrThreads: { min: 1, max: 8 } },
  machine: { cores: 8, memoryBytes: 16 * 1024 ** 3 },
  warnings: [],
  ocr: { reported: false, pages: 0, threads: 0 },
};

async function open(extra = {}, isOwner = true) {
  server = { ...BASE, ...extra };
  api.get.mockImplementation(async (url) => {
    if (url !== '/admin/performance') throw new Error(`unexpected GET ${url}`);
    return { data: server };
  });
  api.put.mockImplementation(async (url, body) => {
    if (url !== '/admin/performance') throw new Error(`unexpected PUT ${url}`);
    const values = { ...server.values };
    const overridden = new Set(server.overridden);
    for (const [key, value] of Object.entries(body)) {
      if (value === null) { values[key] = server.defaults[key]; overridden.delete(key); } else { values[key] = value; overridden.add(key); }
    }
    server = { ...server, values, overridden: [...overridden] };
    return { data: server };
  });
  view = await mount(<PerformanceSection isOwner={isOwner} />);
}
const input = (key) => document.getElementById(`perf-${key}`);
const type = (key, value) => view.type(input(key), String(value));
const text = () => view.text();

beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('PerformanceSection: what it shows', () => {
  it('names the four settings, says the machine, and says the default and the range of each', async () => {
    await open();
    for (const label of ['Leituras pesadas das telas', 'Comparações de duplicatas ao mesmo tempo', 'Páginas lidas ao mesmo tempo pelo OCR', 'Núcleos por página do OCR']) {
      expect(text()).toContain(label);
    }
    expect(text()).toContain('Esta máquina tem 8 núcleos e 16 GiB de memória');
    expect(text()).toContain('de 1 a 20');
    expect(text()).toContain('de 1 a 8');
    expect(text()).toContain('padrão: 8');
    expect(input('catalogReads').value).toBe('8');
    expect(input('catalogReads').min).toBe('1');
    expect(input('catalogReads').max).toBe('20');
  });

  it('says in the singular a machine with one core, and says nothing of memory it cannot tell', async () => {
    await open({ machine: { cores: 1, memoryBytes: 0 }, limits: { ...BASE.limits, ocrPages: { min: 1, max: 1 } } });
    expect(text()).toContain('Esta máquina tem 1 núcleo.');
    expect(text()).not.toContain('GiB de memória');
  });

  it('says what the OCR service uses now, and that a different choice holds from the next PDF', async () => {
    await open({ values: { ...BASE.values, ocrPages: 3 }, overridden: ['ocrPages'], ocr: { reported: true, pages: 1, threads: 1 } });
    expect(text()).toContain('O serviço de OCR está usando agora 1 página ao mesmo tempo, com 1 núcleo cada.');
    expect(text()).toContain('vale a partir do próximo PDF');
  });

  it('says the OCR waits for the next PDF also when only the cores differ', async () => {
    await open({ values: { ...BASE.values, ocrThreads: 2 }, overridden: ['ocrThreads'], ocr: { reported: true, pages: 1, threads: 1 } });
    expect(text()).toContain('vale a partir do próximo PDF');
  });

  it('does not say the OCR waits for the next PDF when it already uses what was chosen', async () => {
    await open({ ocr: { reported: true, pages: 1, threads: 1 } });
    expect(text()).toContain('O serviço de OCR está usando agora');
    expect(text()).not.toContain('vale a partir do próximo PDF');
  });

  it('says nothing of the OCR service when it does not report', async () => {
    await open();
    expect(text()).not.toContain('O serviço de OCR está usando');
  });

  it('shows the server warnings in words', async () => {
    await open({ values: { ...BASE.values, ocrPages: 8, ocrThreads: 2 }, overridden: ['ocrPages', 'ocrThreads'], warnings: ['ocrCores'] });
    expect(text()).toContain('Isso pede mais do que a máquina tem');
    expect(text()).toContain('O OCR pediria 16 núcleos (8 páginas com 2 cada) e a máquina tem 8');
  });

  it('shows the memory warning with the amount', async () => {
    await open({ values: { ...BASE.values, ocrPages: 8 }, overridden: ['ocrPages'], machine: { cores: 16, memoryBytes: 4 * 1024 ** 3 }, warnings: ['ocrMemory'] });
    expect(text()).toContain('mais da metade da memória da máquina');
  });
});

describe('PerformanceSection: the staff that is not the owner', () => {
  it('sees the values, cannot change them, and is told who can', async () => {
    await open({ values: { ...BASE.values, ocrPages: 3 }, overridden: ['ocrPages'] }, false);
    expect(text()).toContain('Só o dono do acervo muda estes valores');
    expect(document.querySelector('input')).toBeNull();
    expect(input('ocrPages').textContent).toBe('3');
    expect(view.button('Salvar')).toBeUndefined();
    expect(text()).not.toContain('voltar ao padrão');
  });
});

describe('PerformanceSection: the owner changes it', () => {
  it('saves only what changed, and says it holds from the next job', async () => {
    await open();
    expect(view.button('Salvar').disabled).toBe(true); // nothing changed
    await type('ocrPages', 3);
    expect(view.button('Salvar').disabled).toBe(false);
    await view.click(view.button('Salvar'));
    await flush();
    expect(api.put).toHaveBeenCalledTimes(1);
    expect(api.put).toHaveBeenCalledWith('/admin/performance', { ocrPages: 3 });
    expect(text()).toContain('Salvo. Vale a partir do próximo trabalho.');
    expect(input('ocrPages').value).toBe('3');
    expect(text()).toContain('padrão: 1, escolhido: 3');
    expect(view.button('Salvar').disabled).toBe(true);
  });

  it('typing the default again is sent as going back to the default', async () => {
    await open({ values: { ...BASE.values, ocrPages: 3 }, overridden: ['ocrPages'] });
    await type('ocrPages', 1);
    await view.click(view.button('Salvar'));
    await flush();
    expect(api.put).toHaveBeenCalledWith('/admin/performance', { ocrPages: null });
  });

  it('goes back to the default of one setting with its own button, at once', async () => {
    await open({ values: { ...BASE.values, ocrPages: 3, dedupeJobs: 2 }, overridden: ['ocrPages', 'dedupeJobs'] });
    const back = [...document.querySelectorAll('button')].filter((b) => b.textContent === 'voltar ao padrão');
    expect(back).toHaveLength(2);
    await view.click(back[0]);
    await flush();
    expect(api.put).toHaveBeenCalledWith('/admin/performance', { dedupeJobs: null }); // the order of the screen: duplicates come before the OCR
    expect(text()).toContain('padrão: 1, escolhido: 3'); // the other one is left as it was
  });

  it('does not let a number outside the range, or nothing, be saved, and says so', async () => {
    await open();
    await type('ocrPages', 99);
    expect(view.button('Salvar').disabled).toBe(true);
    expect(text()).toContain('Use números inteiros dentro das faixas.');
    await type('ocrPages', '');
    expect(view.button('Salvar').disabled).toBe(true);
    await type('ocrPages', 2.5);
    expect(view.button('Salvar').disabled).toBe(true);
    await type('ocrPages', 0);
    expect(view.button('Salvar').disabled).toBe(true);
    await type('ocrPages', 8); // the largest is allowed
    expect(view.button('Salvar').disabled).toBe(false);
  });

  it('warns while typing, before saving', async () => {
    await open();
    expect(text()).not.toContain('Isso pede mais do que a máquina tem');
    await type('ocrPages', 8);
    await type('ocrThreads', 2);
    expect(text()).toContain('O OCR pediria 16 núcleos (8 páginas com 2 cada) e a máquina tem 8');
    await type('ocrThreads', 1);
    expect(text()).not.toContain('Isso pede mais do que a máquina tem'); // 8 pages of 1 core is exactly the 8 cores
  });

  it('typing what is already in use is not a change', async () => {
    await open({ values: { ...BASE.values, ocrPages: 3 }, overridden: ['ocrPages'] });
    await type('ocrPages', 5);
    expect(view.button('Salvar').disabled).toBe(false);
    await type('ocrPages', 3); // back to what is in use
    expect(view.button('Salvar').disabled).toBe(true);
    expect(view.button('Descartar')).toBeUndefined();
  });

  it('what was saved does not keep the box from following the server afterwards', async () => {
    await open();
    await type('ocrPages', 3);
    await view.click(view.button('Salvar'));
    await flush();
    expect(input('ocrPages').value).toBe('3');
    const back = [...document.querySelectorAll('button')].find((b) => b.textContent === 'voltar ao padrão');
    await view.click(back);
    await flush();
    expect(input('ocrPages').value).toBe('1'); // the default is back, and the box shows it
  });

  it('discards what was typed and not saved', async () => {
    await open();
    await type('catalogReads', 15);
    expect(input('catalogReads').value).toBe('15');
    await view.click(view.button('Descartar'));
    expect(input('catalogReads').value).toBe('8');
    expect(api.put).not.toHaveBeenCalled();
  });

  it('says in words when the server refuses, and keeps what was typed', async () => {
    await open();
    api.put.mockRejectedValueOnce({ response: { status: 500, data: 'Error saving the performance settings' } });
    await type('ocrPages', 3);
    await view.click(view.button('Salvar'));
    await flush();
    expect(document.body.querySelector('[role="alert"]').textContent).toBe('Algo deu errado.');
    expect(input('ocrPages').value).toBe('3');
    expect(view.button('Salvar').disabled).toBe(false);
  });

  it('says it when the server cannot be asked', async () => {
    api.get.mockRejectedValue({ response: { status: 500, data: '' } });
    view = await mount(<PerformanceSection isOwner />);
    expect(text()).toContain('Não foi possível carregar os ajustes de desempenho.');
    expect(text()).toContain('Tentar de novo');
  });
});
