import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}));
// The tab asks again while a job runs: quickly here, so the test can watch it end.
vi.mock('../systemLimits', async (original) => ({ ...(await original()), POLL_MS: 20 }));

import { api } from '../../../lib/api';
import { mount, flush } from '../testUtils';
import { SystemTab } from './SystemTab';

let view;
const HOUR = 3600 * 1000;
const ago = (ms) => new Date(Date.now() - ms).toISOString();

const OFF = { enabled: false, packages: [], job: null };
const QUIET = { pending: 0, running: 0, failedRecent: 0, oldestWaiting: null };
const ROOMY = { freeBytes: 80 * 1024 ** 3, totalBytes: 100 * 1024 ** 3 };

async function open({ last = null, queue = QUIET, storage = ROOMY, health = { status: 'ok', components: { database: 'ok', redis: 'ok' } }, backupError, panel = OFF, isOwner = false } = {}) {
  api.get.mockImplementation(async (url) => {
    if (url === '/healthz') return { data: health };
    if (url === '/admin/backup') {
      if (backupError) throw backupError;
      return { data: { lastBackup: last, queue, storage, panel: typeof panel === 'function' ? panel() : panel } };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  view = await mount(<SystemTab isOwner={isOwner} />);
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


// ---- The owner's two buttons (DEC-123) ----

const PKG_A = { name: 'codice-backup-20261004-030000.tar.age', bytes: 712346070, at: '2026-10-04T03:00:00Z', encrypted: true };
const PKG_B = { name: 'codice-backup-20261003-030000.tar.age', bytes: 700000000, at: '2026-10-03T03:00:00Z', encrypted: true };
const ON = (extra = {}) => ({ enabled: true, dir: '/backups', packages: [PKG_A, PKG_B], job: null, ...extra });
const job = (extra = {}) => ({ id: 7, type: 'backup', state: 'succeeded', name: '', startedAt: ago(5 * 60 * 1000), finishedAt: ago(2 * 60 * 1000), ...extra });
const password = () => document.body.querySelector('input[aria-label="Sua senha"]');
const dialog = () => document.body.querySelector('[role="dialog"]');
const submit = async (label, pass = 'minha senha') => {
  await view.type(password(), pass);
  expect(dialog().querySelector('button[type="submit"]').textContent).toBe(label);
  await view.click(dialog().querySelector('button[type="submit"]'));
  await flush();
};
const refusal = (status, data) => Object.assign(new Error(String(status)), { response: { status, data } });

describe('SystemTab: the owner\'s backup buttons', () => {
  it('tells the owner how to set them up when the server is not, and says nothing to an administrator', async () => {
    await open({ isOwner: true });
    expect(text()).toContain('Fazer e verificar backups por aqui');
    expect(text()).toContain('CODICE_BACKUP_DIR');
    expect(text()).toContain('CODICE_BACKUP_PASSPHRASE_FILE');
    expect(view.button('Fazer um backup agora')).toBeUndefined();
    view.unmount();
    await open({ isOwner: false });
    expect(text()).not.toContain('Fazer e verificar backups por aqui');
  });

  it('gives the owner the buttons, the folder and the packages', async () => {
    await open({ isOwner: true, panel: ON() });
    expect(view.button('Fazer um backup agora')).toBeTruthy();
    expect(text()).toContain('/backups');
    expect(text()).toContain(PKG_A.name);
    expect(text()).toContain(PKG_B.name);
    expect(view.container.querySelectorAll('button[aria-label^="Verificar codice-backup"]').length).toBe(2);
    expect(text()).toContain('criptografado');
  });

  it('shows an administrator the packages and no button, and not the folder', async () => {
    await open({ isOwner: false, panel: { ...ON(), dir: undefined } });
    expect(text()).toContain(PKG_A.name);
    expect(view.button('Fazer um backup agora')).toBeUndefined();
    expect(view.button('Verificar')).toBeUndefined();
    expect(text()).not.toContain('Pasta:');
  });

  it('says so when the folder has no package yet', async () => {
    await open({ isOwner: true, panel: ON({ packages: [] }) });
    expect(text()).toContain('Nenhum pacote nesta pasta ainda.');
  });

  it('lists at most the eight newest', async () => {
    const many = Array.from({ length: 12 }, (_, i) => ({ ...PKG_A, name: `codice-backup-202610${String(20 - i).padStart(2, '0')}-030000.tar.age` }));
    await open({ isOwner: true, panel: ON({ packages: many }) });
    expect(view.container.querySelectorAll('button[aria-label^="Verificar codice-backup"]').length).toBe(8);
  });

  it('asks the password again before making a backup, and sends it with the request', async () => {
    api.post.mockResolvedValue({ data: { job_id: 9 } });
    await open({ isOwner: true, panel: ON() });
    await view.click(view.button('Fazer um backup agora'));
    expect(dialog().textContent).toContain('criptografado');
    expect(view.button('Fazer o backup').disabled).toBe(true);
    await submit('Fazer o backup');
    expect(api.post).toHaveBeenCalledWith('/admin/backup/run', { password: 'minha senha' });
    expect(dialog()).toBeNull();
  });

  it('asks the password again before checking a package, and says which one', async () => {
    api.post.mockResolvedValue({ data: { job_id: 9 } });
    await open({ isOwner: true, panel: ON() });
    await view.click(view.container.querySelector(`button[aria-label="Verificar ${PKG_B.name}"]`));
    expect(dialog().textContent).toContain(PKG_B.name);
    expect(dialog().textContent).toContain('banco temporário');
    await submit('Verificar');
    expect(api.post).toHaveBeenCalledWith('/admin/backup/verify', { password: 'minha senha', name: PKG_B.name });
    expect(dialog()).toBeNull();
  });

  it('asks for the password in a field that hides it', async () => {
    await open({ isOwner: true, panel: ON() });
    await view.click(view.button('Fazer um backup agora'));
    expect(password().type).toBe('password');
  });

  it('sends nothing when the form is submitted with no password, or again while it is being sent', async () => {
    let release;
    api.post.mockImplementation(() => new Promise((resolve) => { release = () => resolve({ data: { job_id: 1 } }); }));
    await open({ isOwner: true, panel: ON() });
    await view.click(view.button('Fazer um backup agora'));
    const send = async () => {
      const { act } = await import('react');
      await act(async () => { dialog().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    };
    await send(); // nothing typed
    expect(api.post).not.toHaveBeenCalled();
    await view.type(password(), 'minha senha');
    await send();
    await flush(); // the screen has seen that it is on its way
    await send(); // a second Enter while the first is still out
    expect(api.post).toHaveBeenCalledTimes(1);
    release();
    await flush();
  });

  it('asks the server again after an action, so the new job shows', async () => {
    api.post.mockResolvedValue({ data: { job_id: 9 } });
    await open({ isOwner: true, panel: ON() });
    const before = api.get.mock.calls.filter(([url]) => url === '/admin/backup').length;
    await view.click(view.button('Fazer um backup agora'));
    await submit('Fazer o backup');
    expect(api.get.mock.calls.filter(([url]) => url === '/admin/backup').length).toBeGreaterThan(before);
  });

  it('does not show the folder to someone who is not the owner, even if it came', async () => {
    await open({ isOwner: false, panel: ON({ dir: '/segredo-do-servidor' }) });
    expect(text()).not.toContain('/segredo-do-servidor');
  });

  it('says a wrong password in the dialog, which stays open', async () => {
    api.post.mockRejectedValue(refusal(403, 'The password is not correct'));
    await open({ isOwner: true, panel: ON() });
    await view.click(view.button('Fazer um backup agora'));
    await submit('Fazer o backup', 'errada');
    expect(dialog()).not.toBeNull();
    expect(dialog().textContent).toContain('A senha não está correta.');
  });

  it('closes and says so when another job is already running', async () => {
    api.post.mockRejectedValue(refusal(409, { error: 'A backup job is already running', job_id: 4 }));
    await open({ isOwner: true, panel: ON() });
    await view.click(view.button('Fazer um backup agora'));
    await submit('Fazer o backup');
    expect(dialog()).toBeNull();
    expect(text()).toContain('Já há um trabalho de backup em andamento');
  });

  it('does not send anything when it is cancelled, by the button or Escape', async () => {
    await open({ isOwner: true, panel: ON() });
    await view.click(view.button('Fazer um backup agora'));
    await view.click(view.button('Cancelar'));
    expect(dialog()).toBeNull();
    await view.click(view.button('Fazer um backup agora'));
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await flush();
    expect(dialog()).toBeNull();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('follows a running job: the buttons wait, and the tab asks again until it ends', async () => {
    let state = 'running';
    await open({ isOwner: true, panel: () => ON({ job: job({ state, finishedAt: null }) }) });
    expect(text()).toContain('Fazendo o backup…');
    expect(view.button('Fazer um backup agora').disabled).toBe(true);
    expect(view.container.querySelector('button[aria-label^="Verificar codice-backup"]').disabled).toBe(true);
    state = 'succeeded';
    await new Promise((resolve) => setTimeout(resolve, 150));
    await flush();
    expect(text()).toContain('O backup feito por aqui terminou');
    expect(view.button('Fazer um backup agora').disabled).toBe(false);
  });

  it('says which package is being checked', async () => {
    await open({ isOwner: true, panel: ON({ job: job({ type: 'verify_backup', state: 'pending', name: PKG_A.name, finishedAt: null }) }) });
    expect(text()).toContain(`Verificando ${PKG_A.name}`);
  });

  it('reports a check that passed, for a day, and then lets it go', async () => {
    await open({ isOwner: true, panel: ON({ job: job({ type: 'verify_backup', name: PKG_A.name }) }) });
    expect(text()).toContain(`A verificação de ${PKG_A.name} terminou`);
    expect(text()).toContain('o ensaio de restauração passou');
    view.unmount();
    await open({ isOwner: true, panel: ON({ job: job({ finishedAt: ago(30 * HOUR) }) }) });
    expect(text()).not.toContain('O backup feito por aqui terminou');
  });

  it('says a failure with what went wrong, to the owner, and keeps saying it however old', async () => {
    await open({ isOwner: true, panel: ON({ job: job({ state: 'failed', finishedAt: ago(50 * HOUR), error: 'the passphrase file is missing, unreadable or shorter than 8 characters' }) }) });
    expect(text()).toContain('O backup falhou');
    expect(text()).toContain('O arquivo com a frase de segurança não existe');
  });

  it('sets a text it has no sentence for apart as a technical detail, never as its own words', async () => {
    await open({ isOwner: true, panel: ON({ job: job({ type: 'verify_backup', name: PKG_A.name, state: 'failed', error: 'pg_restore: exit status 1' }) }) });
    expect(text()).toContain(`A verificação de ${PKG_A.name} falhou`);
    expect(text()).toContain('Detalhe técnico: pg_restore: exit status 1');
    expect(text()).not.toContain('falhou agora há pouco. pg_restore');
  });

  it('shows no technical detail when the server has a sentence for the error', async () => {
    await open({ isOwner: true, panel: ON({ job: job({ state: 'failed', error: 'that is not a package in the backup folder' }) }) });
    expect(text()).toContain('Esse pacote não está na pasta de backups.');
    expect(text()).not.toContain('Detalhe técnico');
  });

  it('tells an administrator that it failed without the text of the error', async () => {
    await open({ isOwner: false, panel: { ...ON({ job: job({ state: 'failed' }) }), dir: undefined } });
    expect(text()).toContain('O backup falhou');
  });

  it('says a cancelled job was cancelled', async () => {
    await open({ isOwner: true, panel: ON({ job: job({ state: 'cancelled' }) }) });
    expect(text()).toContain('O backup foi cancelado.');
  });
});
