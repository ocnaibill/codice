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

async function open(state) {
  state = { models: [
    { id: 'intfloat/multilingual-e5-small', name: 'Leve (e5-small)', downloadMB: 471, scope: 'Português e inglês' },
    { id: 'sentence-transformers/LaBSE', name: 'Multilíngue amplo (LaBSE)', downloadMB: 1880, scope: 'Alfabetos distantes' },
  ], ...state };
  api.get.mockResolvedValue({ data: state });
  api.put.mockResolvedValue({ data: { ...state, enabled: true } });
  view = await mount(<EmbeddingsTab />);
}

describe('EmbeddingsTab', () => {
  it('starts off and enables the local model explicitly', async () => {
    await open({ enabled: false, available: true, state: 'idle', model: 'sentence-transformers/LaBSE', error: '' });
    const toggle = view.container.querySelector('input[type="checkbox"]');
    expect(toggle.checked).toBe(false);
    await view.click(toggle);
    expect(api.put).toHaveBeenCalledWith('/admin/embeddings', { enabled: true, model: 'sentence-transformers/LaBSE' });
  });

  it('lets the owner choose the lighter model before enabling it', async () => {
    await open({ enabled: false, available: true, state: 'idle', model: 'sentence-transformers/LaBSE', error: '' });
    const light = [...view.container.querySelectorAll('input[type="radio"]')][0];
    await view.click(light);
    expect(api.put).toHaveBeenCalledWith('/admin/embeddings', { enabled: false, model: 'intfloat/multilingual-e5-small' });
    expect(view.text()).toContain('471 MB');
  });

  it('does not pretend it can enable a worker that is absent', async () => {
    await open({ enabled: false, available: false, state: '', model: '', error: '' });
    expect(view.container.querySelector('input[type="checkbox"]').disabled).toBe(true);
    expect(view.text()).toContain('não está em execução');
  });

  it('shows preparation and worker errors', async () => {
    await open({ enabled: true, available: true, state: 'preparing', model: 'sentence-transformers/LaBSE', error: '' });
    expect(view.text()).toContain('Baixando ou preparando');
    view.unmount();
    await open({ enabled: true, available: true, state: 'error', model: 'sentence-transformers/LaBSE', error: 'download failed' });
    expect(view.text()).toContain('download failed');
  });
});
