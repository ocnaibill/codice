import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { SystemTab } from './SystemTab';

let view;
const HOUR = 3600 * 1000;
const ago = (ms) => new Date(Date.now() - ms).toISOString();

const QUIET = { pending: 0, running: 0, failedRecent: 0, oldestWaiting: null };
const ROOMY = { freeBytes: 80 * 1024 ** 3, totalBytes: 100 * 1024 ** 3 };

async function open({ last = null, queue = QUIET, storage = ROOMY, health = { status: 'ok', components: { database: 'ok', redis: 'ok' } }, backupError } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/healthz') return { data: health };
    if (url === '/admin/backup') {
      if (backupError) throw backupError;
      return { data: { lastBackup: last, queue, storage } };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  view = await mount(<SystemTab />);
}
const text = () => view.text();

beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

const package_ = (extra = {}) => ({
  at: ago(9 * HOUR), bytes: 712346070, files: 28, includesFiles: true, encrypted: true,
  name: 'codice-backup-20261004-030000.tar.age', verified: null, sameDisk: null, ...extra,
});

describe('SystemTab: health', () => {
  it('says the components are working', async () => {
    await open();
    expect(text()).toContain('Banco de dados');
    expect(text()).toContain('Redis');
    expect(text()).toContain('funcionando');
  });

  it('asks the public health with the 503 of a down database counted as an answer', async () => {
    await open({ health: { status: 'down', components: { database: 'down', redis: 'ok' } } });
    expect(api.get).toHaveBeenCalledWith('/healthz', expect.objectContaining({ validateStatus: expect.any(Function) }));
    const options = api.get.mock.calls.find(([url]) => url === '/healthz')[1];
    expect(options.validateStatus(503)).toBe(true);
    expect(text()).toContain('fora do ar: nada funciona sem ele');
  });

  it('says a Redis that is down loses nothing, and one that is off is not used', async () => {
    await open({ health: { status: 'degraded', components: { database: 'ok', redis: 'down' } } });
    expect(text()).toContain('nada se perde, os trabalhos esperam no banco');
    view.unmount();
    await open({ health: { status: 'ok', components: { database: 'ok', redis: 'off' } } });
    expect(text()).toContain('não usado');
  });
});

describe('SystemTab: queue and space', () => {
  it('counts the jobs and calls out the ones that failed', async () => {
    await open({ queue: { pending: 7, running: 2, failedRecent: 3, oldestWaiting: null } });
    expect(text()).toContain('Esperando7');
    expect(text()).toContain('Em andamento2');
    expect(text()).toContain('Falharam nas últimas 24 horas3');
    expect(text()).not.toContain('A fila pode estar parada');
    const red = [...view.container.querySelectorAll('.text-danger')].map((el) => el.textContent);
    expect(red).toContain('3');
  });

  it('does not paint the failures red when there are none', async () => {
    await open();
    expect([...view.container.querySelectorAll('.text-danger')].map((el) => el.textContent)).not.toContain('0');
  });

  it('warns that the queue may be stopped when the oldest job has waited a long time', async () => {
    await open({ queue: { pending: 4, running: 0, failedRecent: 0, oldestWaiting: ago(40 * 60 * 1000) } });
    expect(text()).toContain('A fila pode estar parada');
    expect(text()).toContain('há 40 minutos');
  });

  it('does not warn for a job that has only just been due', async () => {
    await open({ queue: { pending: 1, running: 0, failedRecent: 0, oldestWaiting: ago(5 * 60 * 1000) } });
    expect(text()).not.toContain('A fila pode estar parada');
  });

  it('shows the free space and a bar of how much is used', async () => {
    await open();
    expect(text()).toContain('80,0 GB livres de 100,0 GB (80% livre)');
    const bar = view.container.querySelector('[role="img"]');
    expect(bar.getAttribute('aria-label')).toBe('20% usado');
    expect(bar.firstElementChild.style.width).toBe('20%');
    expect(text()).not.toContain('Pouco espaço livre');
  });

  it('calls out little room, under 15%', async () => {
    await open({ storage: { freeBytes: 10 * 1024 ** 3, totalBytes: 100 * 1024 ** 3 } });
    expect(text()).toContain('Pouco espaço livre');
    view.unmount();
    await open({ storage: { freeBytes: 16 * 1024 ** 3, totalBytes: 100 * 1024 ** 3 } });
    expect(text()).not.toContain('Pouco espaço livre');
  });

  it('says so when the server could not measure the space', async () => {
    await open({ storage: null });
    expect(text()).toContain('O servidor não soube medir o espaço.');
  });
});

describe('SystemTab: backup', () => {
  it('says, in red, when no backup was ever made, and still gives the command to make one', async () => {
    await open();
    expect(text()).toContain('Nenhum backup registrado nesta instância');
    expect(view.container.querySelector('input[aria-label="Fazer um backup (no servidor)"]').value).toContain('scripts/backup.sh');
  });

  it('shows the latest backup, how old it is and what it holds', async () => {
    await open({ last: package_() });
    expect(text()).toContain('Último backup');
    expect(text()).toContain('há 9 horas');
    expect(text()).toContain('679,3 MB');
    expect(text()).toContain('com 28 arquivo(s)');
    expect(text()).toContain('criptografado');
    expect(text()).not.toContain('O último backup é velho');
  });

  it('calls out a backup older than a day and a half, and not one just inside it', async () => {
    await open({ last: package_({ at: ago(37 * HOUR), includesFiles: false, encrypted: false }) });
    expect(text()).toContain('O último backup é velho');
    expect(text()).toContain('só banco e lista de arquivos');
    expect(text()).toContain('sem criptografia');
    view.unmount();
    await open({ last: package_({ at: ago(35 * HOUR) }) });
    expect(text()).not.toContain('O último backup é velho');
  });

  it('shows the owner the path, and the administrator only the name', async () => {
    await open({ last: package_({ path: '/mnt/backups/codice/codice-backup-20261004-030000.tar.age' }) });
    expect(text()).toContain('/mnt/backups/codice/codice-backup-20261004-030000.tar.age');
    view.unmount();
    await open({ last: package_() });
    expect(text()).toContain('codice-backup-20261004-030000.tar.age');
    expect(text()).not.toContain('/mnt/');
  });

  it('tells how to record where a streamed package is, when it has no address', async () => {
    await open({ last: package_({ name: undefined }) });
    expect(text()).toContain('o Códice não sabe onde ele está');
    expect(text()).toContain('--as-path');
  });

  it('says whether this package was checked, and how far', async () => {
    await open({ last: package_() });
    expect(text()).toContain('ainda não foi verificado');
    view.unmount();
    await open({ last: package_({ verified: { at: ago(2 * HOUR), deep: false } }) });
    expect(text()).toContain('o pacote foi lido inteiro e confere. Falta o ensaio de restauração');
    view.unmount();
    await open({ last: package_({ verified: { at: ago(2 * HOUR), deep: true } }) });
    expect(text()).toContain('o ensaio de restauração passou');
    expect(text()).not.toContain('ainda não foi verificado');
  });

  it('warns when the package is on the same disk as the storage, and only then', async () => {
    await open({ last: package_({ sameDisk: true }) });
    expect(text()).toContain('O pacote está no mesmo disco do acervo');
    expect(text()).not.toContain('não enxerga o disco');
    view.unmount();
    await open({ last: package_({ sameDisk: false }) });
    expect(text()).not.toContain('mesmo disco do acervo');
    expect(text()).not.toContain('não enxerga o disco');
    view.unmount();
    await open({ last: package_({ sameDisk: null }) });
    expect(text()).toContain('não enxerga o disco onde o pacote está');
    expect(text()).not.toContain('mesmo disco do acervo');
  });

  it('gives the restore command to copy, and says why it is not done from here', async () => {
    await open({ last: package_() });
    const restore = view.container.querySelector('input[aria-label="Restaurar um pacote (no servidor)"]');
    expect(restore.value).toContain('codice-admin restore --in -');
    expect(text()).toContain('Restaurar troca o banco que está no ar');
  });

  it('copies a command', async () => {
    const writeText = vi.fn().mockResolvedValue();
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    await open({ last: package_() });
    await view.click(view.container.querySelector('button[aria-label="Copiar restaurar um pacote (no servidor)"]'));
    await flush();
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('codice-admin restore'));
    expect(text()).toContain('Copiado');
  });
});

describe('SystemTab: when it does not load', () => {
  it('says so and offers to try again', async () => {
    await open({ backupError: new Error('down') });
    expect(text()).toContain('Não foi possível carregar o estado do sistema');
    expect(view.button('Tentar de novo')).toBeTruthy();
  });

  it('says there is no permission for a 403, without offering to try again', async () => {
    const forbidden = Object.assign(new Error('403'), { response: { status: 403 } });
    await open({ backupError: forbidden });
    expect(text()).toContain('Você não tem permissão para ver isto');
  });
});
