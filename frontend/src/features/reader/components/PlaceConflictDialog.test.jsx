import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { PlaceConflictDialog } from './PlaceConflictDialog';

let view;
const pending = { device: 'Kindle de Binaco', percent: 64, at: new Date(Date.now() - 18 * 60000).toISOString(), locator: { type: 'epub', cfi: 'epubcfi(/6/36)' } };
const web = { percent: 50, updatedAt: new Date(Date.now() - 20 * 3600 * 1000).toISOString() };
const comparison = {
  web: { percent: 50, at: web.updatedAt, device: 'Web', chapter: 'Livro II: Capítulo 14', snippet: 'A caminhada noturna pelas dunas.' },
  device: { percent: 64, at: pending.at, device: 'Kindle de Binaco', chapter: 'Livro II: Capítulo 18', snippet: 'O homem que consegue estar em dois lugares.' },
  gapPercent: 14, notes: 42,
};

async function open(extra = {}) {
  const props = { fileId: 7, title: 'Duna', pending, web, onGoTo: vi.fn(), onStay: vi.fn(), ...extra };
  view = await mount(<PlaceConflictDialog {...props} />);
  return props;
}
afterEach(() => { view?.unmount(); view = null; });
beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ data: comparison });
});

describe('PlaceConflictDialog', () => {
  it('puts both places side by side, each in words', async () => {
    await open();
    expect(api.get).toHaveBeenCalledWith('/progress/files/7/compare');
    const text = view.dialog().textContent;
    expect(text).toContain('Conflito de progresso detectado');
    expect(text).toContain('Duna');
    expect(text).toContain('Divergência de 14%');
    expect(text).toContain('Kindle de Binaco');
    expect(text).toContain('Livro II: Capítulo 18');
    expect(text).toContain('Livro II: Capítulo 14');
    expect(text).toContain('O homem que consegue estar em dois lugares.');
    expect(text).toContain('Há 18 minutos');
    expect(text).toContain('42 anotações neste livro');
  });

  it('goes to the place of the device or stays on the one of the web', async () => {
    const props = await open();
    await view.click(view.buttonMatching(/Avançar para 64%/));
    expect(props.onGoTo).toHaveBeenCalledWith(pending.locator);
    await view.click(view.buttonMatching(/Permanecer em 50%/));
    expect(props.onStay).toHaveBeenCalled();
  });

  it('closing is staying', async () => {
    const props = await open();
    await view.click(view.dialog().querySelector('button[aria-label^="Fechar"]'));
    expect(props.onStay).toHaveBeenCalled();
  });

  it('says "Voltar" when the device is behind the web', async () => {
    api.get.mockResolvedValue({ data: { ...comparison, device: { ...comparison.device, percent: 20 }, gapPercent: 30 } });
    await open({ pending: { ...pending, percent: 20 }, web: { ...web, percent: 50 } });
    expect(view.buttonMatching(/Voltar para 20%/)).toBeTruthy();
  });

  it('draws what it knows at once and makes up nothing the server did not say', async () => {
    api.get.mockRejectedValue({ response: { status: 500 } });
    await open();
    const text = view.dialog().textContent;
    expect(text).toContain('64%');
    expect(text).toContain('50%');
    expect(text).toContain('Divergência de 14%');
    expect(text).not.toContain('Página');
    expect(text).not.toContain('anotações neste livro');
    expect(text).not.toContain('Trecho onde');
  });

  it('says the pages when the server has them', async () => {
    api.get.mockResolvedValue({ data: { ...comparison, web: { ...comparison.web, page: 204 }, device: { ...comparison.device, page: 262 }, gapPages: 58 } });
    await open();
    const text = view.dialog().textContent;
    expect(text).toContain('Página 262');
    expect(text).toContain('Página 204');
    expect(text).toContain('(+58 págs.)');
  });
});
