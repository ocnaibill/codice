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
    expect(api.put).toHaveBeenCalledWith('/admin/embeddings', { enabled: true });
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
