import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { ProvidersTab } from './ProvidersTab';

const provider = (id, name, enabled, over = {}) => ({ id, name, enabled, sends: ['title'], needsKey: false, ...over });
const providers = [
  provider('google_books', 'Google Books', false),
  provider('openlibrary', 'Open Library', true),
  provider('comicvine', 'ComicVine', false, { needsKey: true }),
];

let view;
async function open(props = { isOwner: true }, data = providers) {
  api.get.mockImplementation(async (url) => {
    if (url === '/admin/metadata-providers') {
      if (data instanceof Error) throw data;
      return { data: { data } };
    }
    throw new Error(`unexpected GET ${url}`);
  });
  api.put.mockResolvedValue({});
  view = await mount(<ProvidersTab {...props} />);
}
const box = (name) => document.body.querySelector(`input[aria-label^="${name}:"]`);
const confirmDialog = () => document.body.querySelector('[role="dialog"]');
beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('ProvidersTab: which external services may be asked (#68)', () => {
  it('says for each provider whether it is on, what it receives and where it lives', async () => {
    await open();
    const text = view.text();
    expect(text).toContain('Todos começam desligados');
    expect(text).toContain('Open Library');
    expect(text).toContain('Recebe: o título da obra. Endereço: openlibrary.org (Internet Archive).');
    expect(text).toContain('Endereço: googleapis.com (Google).');
    expect(text).toContain('COMICVINE_API_KEY');
    expect([...document.body.querySelectorAll('li span.font-mono')].map((e) => e.textContent)).toEqual(['desligado', 'ligado', 'desligado']);
    expect(box('Open Library').checked).toBe(true);
    expect(box('Google Books').checked).toBe(false);
    expect(box('ComicVine').checked).toBe(false);
  });

  it('turns one on only after saying what is sent, and not at all if the owner backs out', async () => {
    await open();
    await view.click(box('Google Books'));
    expect(api.put).not.toHaveBeenCalled();
    const dialogText = confirmDialog().textContent;
    expect(dialogText).toContain('Ligar Google Books?');
    expect(dialogText).toContain('googleapis.com (Google)');
    expect(dialogText).toContain('O nome do arquivo, o autor, o conteúdo do livro e as notas não são enviados');
    expect(dialogText).toContain('chave de API do Google Books');

    await view.click(view.button('Cancelar'));
    expect(api.put).not.toHaveBeenCalled();
    expect(confirmDialog()).toBeNull();

    await view.click(box('Google Books'));
    await view.click([...document.body.querySelectorAll('[role="dialog"] button')].find((b) => b.textContent === 'Ligar'));
    expect(api.put).toHaveBeenCalledWith('/admin/metadata-providers/google_books', { enabled: true });
  });

  it('turns one off at once, with nothing to confirm', async () => {
    await open();
    await view.click(box('Open Library'));
    expect(confirmDialog()).toBeNull();
    expect(api.put).toHaveBeenCalledWith('/admin/metadata-providers/openlibrary', { enabled: false });
  });

  it('shows the choice to an admin but lets only the owner change it', async () => {
    await open({ isOwner: false });
    expect(box('Open Library').checked).toBe(true);
    for (const name of ['Google Books', 'Open Library', 'ComicVine']) expect(box(name).disabled).toBe(true);
    expect(view.text()).toContain('Só o owner liga ou desliga os provedores.');
  });

  it('does not say that only the owner changes it to the owner', async () => {
    await open({ isOwner: true });
    expect(view.text()).not.toContain('Só o owner liga ou desliga');
    expect(box('Google Books').disabled).toBe(false);
  });

  it('shows what the server said when the change fails', async () => {
    await open();
    api.put.mockRejectedValue({ response: { status: 403, data: 'Owner only' } });
    await view.click(box('Open Library'));
    expect(view.text()).toContain('Owner only');
  });

  it('says so when it could not load, and when there is nothing to choose', async () => {
    await open({ isOwner: true }, new Error('offline'));
    expect(view.text()).toContain('Não foi possível carregar os provedores.');
    expect(view.text()).not.toContain('Nenhum provedor.');
    view.unmount();
    await open({ isOwner: true }, []);
    expect(view.text()).toContain('Nenhum provedor.');
  });

  it('names a provider it knows nothing about plainly, without an address', async () => {
    await open({ isOwner: true }, [provider('novo', 'Novo', false, { sends: ['title', 'author'] })]);
    expect(view.text()).toContain('Recebe: o título da obra, author. Endereço: —.');
  });
});
