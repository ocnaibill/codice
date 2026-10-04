import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount } from '../../features/admin/testUtils';
import { AboutModal } from './AboutModal';

let view;
const ABOUT = {
  version: '6cd17ef',
  license: 'AGPL-3.0',
  licenseUrl: 'https://www.gnu.org/licenses/agpl-3.0.html',
  sourceUrl: 'https://git.example.com/me/codice',
};

async function open(reply, onClose = vi.fn()) {
  api.get.mockImplementation(async (url) => {
    if (url !== '/about') throw new Error(`unexpected GET ${url}`);
    if (reply instanceof Error) throw reply;
    return { data: reply };
  });
  view = await mount(<AboutModal onClose={onClose} />);
  return onClose;
}
const link = (label) => [...document.body.querySelectorAll('a')].find((a) => a.textContent === label);

beforeEach(() => vi.clearAllMocks());
afterEach(() => view.unmount());

describe('AboutModal', () => {
  it('says the version, the license, and links the source and the license text', async () => {
    await open(ABOUT);
    const text = view.dialog().textContent;
    expect(text).toContain('Versão 6cd17ef');
    expect(text).toContain('GNU AGPLv3');
    expect(text).toContain('direito ao código-fonte');
    expect(link('Código-fonte').href).toBe('https://git.example.com/me/codice');
    expect(link('Texto da licença').href).toBe('https://www.gnu.org/licenses/agpl-3.0.html');
  });

  it('opens every link in another tab without handing over the page', async () => {
    await open(ABOUT);
    const links = [...view.dialog().querySelectorAll('a')];
    expect(links.length).toBeGreaterThan(5);
    for (const a of links) {
      expect(a.target).toBe('_blank');
      expect(a.rel).toContain('noopener');
      expect(a.rel).toContain('noreferrer');
    }
  });

  it('calls a build with no version a development version', async () => {
    await open({ ...ABOUT, version: 'dev' });
    expect(view.dialog().textContent).toContain('Versão de desenvolvimento');
    expect(view.dialog().textContent).not.toContain('Versão dev');
  });

  it('never turns an address that is not http(s) into a link', async () => {
    await open({ ...ABOUT, sourceUrl: 'javascript:alert(1)', licenseUrl: 'data:text/html,x' });
    expect(link('Código-fonte')).toBeUndefined();
    expect(link('Texto da licença')).toBeUndefined();
    expect(document.body.querySelector('a[href^="javascript"]')).toBeNull();
  });

  it('names what it carries: the fonts and the Wiktionary with its license', async () => {
    await open(ABOUT);
    const text = view.dialog().textContent;
    for (const name of ['OpenDyslexic', 'Newsreader', 'epub.js', 'PDF.js', 'Wikcionário']) expect(text).toContain(name);
    expect(text).toContain('SIL OFL 1.1');
    expect(text).toContain('CC BY-SA 4.0 e GFDL');
  });

  it('shows no contact of the owner and nothing about the instance', async () => {
    await open(ABOUT);
    expect(view.dialog().textContent).not.toMatch(/contato|e-mail|@/i);
  });

  it('keeps the third-party list when the server does not answer, and offers to try again', async () => {
    const error = new Error('down');
    await open(error);
    expect(view.dialog().textContent).toContain('Não foi possível carregar os dados desta instalação');
    expect(view.dialog().textContent).toContain('OpenDyslexic');
    api.get.mockResolvedValue({ data: ABOUT });
    await view.click(view.button('Tentar de novo'));
    expect(view.dialog().textContent).toContain('Versão 6cd17ef');
  });

  it('closes with the button, with Escape and by clicking outside', async () => {
    const onClose = await open(ABOUT);
    await view.click(view.button('Fechar'));
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await view.click(view.dialog().parentElement);
    expect(onClose).toHaveBeenCalledTimes(3);
  });
});
