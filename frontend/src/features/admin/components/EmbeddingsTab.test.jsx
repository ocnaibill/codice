import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { EmbeddingsTab } from './EmbeddingsTab';

let view;
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

const entry = (over) => ({
  id: 'x', kind: 'semantic', name: 'Modelo', scope: 'Escopo', sizeMB: 100, selected: false, state: 'off', inUse: false,
  usage: { count: 0, unit: 'arquivos preparados' }, ...over,
});
const E5 = 'intfloat/multilingual-e5-small';
const LABSE = 'sentence-transformers/LaBSE';

function modelsOf(over = {}) {
  return {
    controls: { semantic: 'none', comicOcr: 'none', ...(over.controls || {}) },
    services: { semantic: true, comicOcr: true, ...(over.services || {}) },
    models: over.models || [
      entry({ id: E5, name: 'Leve (e5-small)', scope: 'Português e inglês', sizeMB: 471 }),
      entry({ id: LABSE, name: 'Multilíngue amplo (LaBSE)', scope: 'Alfabetos distantes', sizeMB: 1880 }),
      entry({ id: 'mobile', kind: 'comic_ocr', name: 'Leve (PaddleOCR mobile)', scope: 'Quadrinhos e mangás', sizeMB: 13, usage: { count: 0, unit: 'páginas lidas' } }),
      entry({ id: 'server', kind: 'comic_ocr', name: 'Servidor (PaddleOCR médio)', scope: 'Mais fiel', sizeMB: 132, usage: { count: 0, unit: 'páginas lidas' } }),
    ],
  };
}

async function open(state = modelsOf()) {
  api.get.mockResolvedValue({ data: state });
  api.put.mockResolvedValue({ data: state });
  view = await mount(<EmbeddingsTab />);
}

const radio = (group, label) => [...view.container.querySelectorAll(`input[type="radio"][name="${group}"]`)].find((r) => r.closest('label').textContent.includes(label));

describe('IA local: the choices', () => {
  it('offers none among the choices of each use, and starts with none chosen', async () => {
    await open();
    expect(radio('semantic-model', 'Nenhum').checked).toBe(true);
    expect(radio('comic-ocr-model', 'Nenhum').checked).toBe(true);
    expect(view.container.querySelectorAll('input[name="comic-ocr-model"]')).toHaveLength(3);
    expect(view.container.querySelectorAll('input[name="semantic-model"]')).toHaveLength(3);
  });

  it('says what each model is and how much it downloads', async () => {
    await open();
    expect(view.text()).toContain('471 MB');
    expect(view.text()).toContain('1880 MB');
    expect(view.text()).toContain('13 MB');
    expect(view.text()).toContain('132 MB');
  });

  it('chooses the model that reads comics, with its own request, apart from the semantic one', async () => {
    await open();
    await view.click(radio('comic-ocr-model', 'Servidor'));
    expect(api.put).toHaveBeenCalledWith('/admin/models/comic-ocr', { model: 'server' });
    expect(api.put).not.toHaveBeenCalledWith('/admin/embeddings', expect.anything());
  });

  it('turns the reading of comics off by choosing none', async () => {
    await open(modelsOf({ controls: { comicOcr: 'mobile' }, models: undefined }));
    await view.click(radio('comic-ocr-model', 'Nenhum'));
    expect(api.put).toHaveBeenCalledWith('/admin/models/comic-ocr', { model: 'none' });
  });

  it('turns the semantic matching on with the model chosen, and off by choosing none, keeping the model that was on', async () => {
    await open();
    await view.click(radio('semantic-model', 'Leve (e5-small)'));
    expect(api.put).toHaveBeenCalledWith('/admin/embeddings', { enabled: true, model: E5 });
    view.unmount();
    const on = modelsOf({ controls: { semantic: LABSE } });
    on.models[1] = { ...on.models[1], selected: true, state: 'ready', inUse: true };
    await open(on);
    await view.click(radio('semantic-model', 'Nenhum'));
    expect(api.put).toHaveBeenLastCalledWith('/admin/embeddings', { enabled: false, model: LABSE });
  });

  it('does not offer a model of a service that is not running, and says what to do', async () => {
    await open(modelsOf({ services: { comicOcr: false, semantic: false } }));
    expect(radio('comic-ocr-model', 'Servidor').disabled).toBe(true);
    expect(radio('semantic-model', 'Leve').disabled).toBe(true);
    expect(radio('comic-ocr-model', 'Nenhum').disabled).toBe(false);
    expect(view.text()).toContain('comic-ocr');
    expect(view.text()).toContain('worker de embeddings não está em execução');
  });

  it('says what the server refused', async () => {
    await open();
    api.put.mockRejectedValue({ response: { status: 409, data: 'O serviço de OCR de quadrinhos não está em execução.\n' } });
    await view.click(radio('comic-ocr-model', 'Leve (PaddleOCR mobile)'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('não está em execução');
  });
});

describe('IA local: reading again with the chosen model', () => {
  const rereadButton = () => [...view.container.querySelectorAll('button')].find((b) => b.textContent.startsWith('Reler'));
  const dialog = () => document.querySelector('[role="dialog"]');
  const dialogButton = (label) => [...dialog().querySelectorAll('button')].find((b) => b.textContent === label);
  const withOthers = (pages, over = {}) => ({ ...modelsOf({ controls: { comicOcr: 'server' }, ...over }), comicOcr: { otherModelPages: pages } });

  it('offers nothing when no page was read by another model, or when no model is chosen', async () => {
    await open(withOthers(0));
    expect(rereadButton()).toBeUndefined();
    view.unmount();
    await open({ ...modelsOf(), comicOcr: { otherModelPages: 40 } });
    expect(rereadButton()).toBeUndefined();
  });

  it('says how many pages another model read, and asks before reading them again', async () => {
    await open(withOthers(1234));
    expect(view.text()).toContain('1.234 páginas foram lidas com outro modelo');
    expect(rereadButton().textContent).toBe('Reler 1.234 páginas com este modelo');
    await view.click(rereadButton());
    expect(api.post).not.toHaveBeenCalled();
    expect(dialog().textContent).toContain('Servidor (PaddleOCR médio)');
    expect(dialog().textContent).toContain('o texto que está na busca só é trocado quando a leitura nova termina');
    await view.click(dialogButton('Reler as páginas'));
    expect(api.post).toHaveBeenCalledTimes(1);
    expect(api.post).toHaveBeenCalledWith('/admin/models/comic-ocr/reread');
  });

  it('does not read again when the question is cancelled', async () => {
    await open(withOthers(3));
    await view.click(rereadButton());
    await view.click(dialogButton('Cancelar'));
    expect(dialog()).toBeNull();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('speaks of one page in the singular', async () => {
    await open(withOthers(1));
    expect(view.text()).toContain('1 página foi lida com outro modelo');
    expect(rereadButton().textContent).toBe('Reler 1 página com este modelo');
  });

  it('is not offered while the service that reads is not running', async () => {
    await open(withOthers(5, { services: { comicOcr: false } }));
    expect(rereadButton().disabled).toBe(true);
  });

  it('says what the server refused', async () => {
    await open(withOthers(5));
    api.post.mockRejectedValue({ response: { status: 409, data: 'O serviço de OCR de quadrinhos não está em execução.\n' } });
    await view.click(rereadButton());
    await view.click(dialogButton('Reler as páginas'));
    expect(view.container.querySelector('[role="alert"]').textContent).toContain('não está em execução');
  });
});

describe('IA local: the models and their state', () => {
  const row = (name) => [...view.container.querySelectorAll('ul[aria-label="Modelos locais"] li')].find((li) => li.textContent.includes(name));

  it('says the percentage of a download, with a bar that a screen reader can read', async () => {
    const state = modelsOf({ controls: { comicOcr: 'server' } });
    state.models[3] = { ...state.models[3], selected: true, state: 'downloading', progress: 37 };
    await open(state);
    expect(row('Servidor').textContent).toContain('Baixando 37%');
    const bar = row('Servidor').querySelector('[role="progressbar"]');
    expect(bar.getAttribute('aria-valuenow')).toBe('37');
  });

  it('says which model is in use and how much it was used', async () => {
    const state = modelsOf({ controls: { comicOcr: 'mobile' } });
    state.models[2] = { ...state.models[2], selected: true, state: 'ready', inUse: true, usage: { count: 1234, unit: 'páginas lidas' } };
    await open(state);
    expect(row('Leve (PaddleOCR mobile)').textContent).toContain('Pronto, em uso');
    expect(row('Leve (PaddleOCR mobile)').textContent).toContain('1.234 páginas lidas');
    expect(row('Servidor').textContent).toContain('Desativado');
  });

  it('says a service that is not running and a model that failed, with why', async () => {
    const state = modelsOf({ controls: { comicOcr: 'mobile', semantic: LABSE } });
    state.models[2] = { ...state.models[2], selected: true, state: 'error', error: 'sem memória' };
    state.models[1] = { ...state.models[1], selected: true, state: 'service_missing' };
    await open(state);
    expect(row('Leve (PaddleOCR mobile)').textContent).toContain('Com erro');
    expect(row('Leve (PaddleOCR mobile)').textContent).toContain('sem memória');
    expect(row('LaBSE').textContent).toContain('Serviço parado');
  });

  it('says it could not load them', async () => {
    api.get.mockRejectedValue(new Error('boom'));
    view = await mount(<EmbeddingsTab />);
    expect(view.text()).toContain('Não foi possível carregar os modelos');
  });
});
