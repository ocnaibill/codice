import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { JobsTab } from './JobsTab';

const job = (over) => ({ id: 1, type: 'ingest', workTitle: 'Duna', state: 'failed', attempts: 3, maxAttempts: 3,
  createdAt: '2026-09-01T10:00:00Z', ...over });
let view;

async function open(jobs, counts = {}) {
  api.get.mockResolvedValue({ data: { data: jobs, counts } });
  api.post.mockResolvedValue({});
  view = await mount(<JobsTab />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('JobsTab', () => {
  it('lists the jobs with what they did and why one failed', async () => {
    await open([job({ lastError: 'arquivo corrompido' })], { failed: 1 });
    expect(view.text()).toContain('Leitura do arquivo');
    expect(view.text()).toContain('Duna');
    expect(view.text()).toContain('arquivo corrompido');
    expect(view.buttonMatching(/Com falha\s*\(1\)/)).toBeTruthy();
  });

  it('retries a failed job', async () => {
    await open([job({ id: 7 })]);
    await view.click(view.button('Tentar de novo'));
    expect(api.post).toHaveBeenCalledWith('/admin/jobs/7/rerun');
  });

  it('offers to cancel a job that is waiting or running, not one that is done', async () => {
    await open([job({ id: 8, state: 'running' }), job({ id: 9, state: 'succeeded' })]);
    expect(view.container.querySelectorAll('button').length).toBeGreaterThan(0);
    await view.click(view.button('Cancelar'));
    expect(api.post).toHaveBeenCalledWith('/admin/jobs/8/cancel');
    expect(api.post).toHaveBeenCalledTimes(1);
  });

  it('asks the server for one state when a filter is chosen', async () => {
    await open([job()]);
    await view.click(view.buttonMatching(/^Com falha/));
    expect(api.get).toHaveBeenLastCalledWith('/admin/jobs', { params: { state: 'failed' } });
  });

  it('says so when there is nothing to show', async () => {
    await open([]);
    expect(view.text()).toContain('Nenhum trabalho');
  });
});
