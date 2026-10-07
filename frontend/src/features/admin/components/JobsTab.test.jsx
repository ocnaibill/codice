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

  it('names the job that reads the text of a work, apart from the one that reads the file', async () => {
    await open([job({ id: 3, type: 'extract_text', state: 'pending' })]);
    expect(view.text()).toContain('Leitura do texto para busca');
    expect(view.text()).not.toContain('Leitura do arquivo');
  });

  it('names the owner\'s backup jobs in Portuguese', async () => {
    await open([job({ id: 4, type: 'backup', workTitle: null, state: 'running' }), job({ id: 5, type: 'verify_backup', workTitle: null, state: 'pending' })]);
    expect(view.text()).toContain('Backup');
    expect(view.text()).toContain('Verificação de backup');
    expect(view.text()).not.toContain('verify_backup');
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

  describe('trying all the failed ones again', () => {
    const onFailed = async (counts = { failed: 5 }) => {
      await open([job({ id: 7 })], counts);
      await view.click(view.buttonMatching(/^Com falha/));
    };

    it('has no such button outside the failed filter, or when nothing failed', async () => {
      await open([job()], { failed: 5 });
      expect(view.buttonMatching(/Tentar todos/)).toBeUndefined();
      await view.click(view.buttonMatching(/^Com falha/));
      expect(view.buttonMatching(/Tentar todos de novo \(5\)/)).toBeDefined();
      view.unmount();
      await open([job({ state: 'succeeded' })], { failed: 0, succeeded: 1 });
      await view.click(view.buttonMatching(/^Concluídos/));
      expect(view.buttonMatching(/Tentar todos/)).toBeUndefined();
      await view.click(view.buttonMatching(/^Com falha/));
      expect(view.buttonMatching(/Tentar todos/)).toBeUndefined();   // on the failed filter, but nothing failed
    });

    it('asks first, and does nothing when it is cancelled', async () => {
      await onFailed();
      await view.click(view.buttonMatching(/Tentar todos de novo/));
      expect(view.dialog().textContent).toContain('Tentar de novo todos os trabalhos com falha?');
      expect(api.post).not.toHaveBeenCalled();
      await view.click(view.button('Cancelar'));
      expect(view.dialog()).toBeNull();
      expect(api.post).not.toHaveBeenCalled();
    });

    it('sends the request once it is confirmed, and says how many went back and how many are left', async () => {
      await onFailed();
      api.post.mockResolvedValue({ data: { requeued: 3, left: 2 } });
      await view.click(view.buttonMatching(/Tentar todos de novo/));
      await view.click(view.button('Tentar todos de novo'));
      expect(view.dialog()).toBeNull();
      expect(api.post).toHaveBeenCalledWith('/admin/jobs/rerun-failed');
      expect(view.text()).toContain('3 trabalhos voltaram para a fila.');
      expect(view.text()).toContain('2 continuam com falha');
    });

    it('speaks in the singular, and says so when nothing could go back', async () => {
      await onFailed();
      api.post.mockResolvedValue({ data: { requeued: 1, left: 1 } });
      await view.click(view.buttonMatching(/Tentar todos de novo/));
      await view.click(view.button('Tentar todos de novo'));
      expect(view.text()).toContain('1 trabalho voltou para a fila.');
      expect(view.text()).toContain('1 continua com falha');
      expect(view.text()).toContain('Tente-o um a um');
      view.unmount();

      await onFailed();
      api.post.mockResolvedValue({ data: { requeued: 0, left: 0 } });
      await view.click(view.buttonMatching(/Tentar todos de novo/));
      await view.click(view.button('Tentar todos de novo'));
      expect(view.text()).toContain('Nenhum trabalho voltou para a fila.');
      expect(view.text()).not.toContain('continua');
    });

    it('says when the server refuses', async () => {
      await onFailed();
      api.post.mockRejectedValue({ response: { status: 403, data: 'Forbidden\n' } });
      await view.click(view.buttonMatching(/Tentar todos de novo/));
      await view.click(view.button('Tentar todos de novo'));
      expect(view.text()).toContain('Você não tem permissão para isso.');
    });
  });
});
