import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount } from '../../features/admin/testUtils';
import { PairReaderModal } from './PairReaderModal';
import { formatPairCode, isPairCode } from '../../features/auth/api/useDevicePairing';

let view;
const request = { userCode: 'BCDF-2345', name: 'Kindle da sala', platform: 'kindle', client: 'codice-ereader/0.1', ip: '203.0.113.7', requestedAt: new Date(Date.now() - 3 * 60000).toISOString() };

async function open(props = {}) {
  api.get.mockResolvedValue({ data: request });
  api.post.mockResolvedValue({ data: '' });
  view = await mount(<PairReaderModal onClose={props.onClose ?? vi.fn()} {...props} />);
}
afterEach(() => { view?.unmount(); view = null; });
beforeEach(() => vi.clearAllMocks());

describe('the code', () => {
  it('is written in capitals, in two groups of four', () => {
    expect(formatPairCode('bcdf2345')).toBe('BCDF-2345');
    expect(formatPairCode('bc')).toBe('BC');
    expect(formatPairCode('bcdf 2345 99')).toBe('BCDF-2345');
    expect(isPairCode('BCDF-2345')).toBe(true);
    expect(isPairCode('BCDF-234')).toBe(false);
  });
});

describe('PairReaderModal', () => {
  it('asks for the code, and looks the request up only when it is whole', async () => {
    await open();
    const input = view.dialog().querySelector('input');
    expect(view.button('Procurar').disabled).toBe(true);
    await view.type(input, 'bcdf2345');
    expect(input.value).toBe('BCDF-2345');
    await view.click(view.button('Procurar'));
    expect(api.get).toHaveBeenCalledWith('/auth/device/BCDF2345');
    expect(view.dialog().querySelector('input').value).toBe('Kindle da sala'); // the name the device suggested, to keep or change
    expect(view.dialog().textContent).toContain('203.0.113.7');
    expect(view.dialog().textContent).toContain('agora há pouco'); // 3 minutes: "ago" says nothing for the last few
  });

  it('opens on the request when it comes with the code of the link', async () => {
    await open({ initialCode: 'BCDF2345' });
    expect(api.get).toHaveBeenCalledWith('/auth/device/BCDF2345');
    expect(view.dialog().textContent).toContain('Este leitor pediu para entrar na sua conta');
    expect(view.dialog().textContent).toContain('Aprove só se este mesmo código está, agora, na tela do seu leitor');
  });

  it('approves with the name the person gave', async () => {
    await open({ initialCode: 'BCDF2345' });
    await view.type(view.dialog().querySelector('input'), 'Kindle da Ana');
    await view.click(view.button('Aprovar'));
    expect(api.post).toHaveBeenCalledWith('/auth/device/BCDF2345/approve', { name: 'Kindle da Ana' });
    expect(view.dialog().textContent).toContain('Kindle da Ana');
    expect(view.dialog().textContent).toContain('Pronto');
  });

  it('turns the request down', async () => {
    await open({ initialCode: 'BCDF2345' });
    await view.click(view.button('Negar'));
    expect(api.post).toHaveBeenCalledWith('/auth/device/BCDF2345/deny');
    expect(view.dialog().textContent).toContain('não recebeu acesso nenhum');
  });

  it('says in words that the request is not there or has expired', async () => {
    api.get.mockRejectedValue({ response: { status: 404, data: 'Pedido não encontrado ou vencido.' } });
    api.post.mockResolvedValue({ data: '' });
    view = await mount(<PairReaderModal onClose={vi.fn()} />);
    await view.type(view.dialog().querySelector('input'), 'BCDF2345');
    await view.click(view.button('Procurar'));
    expect(view.dialog().textContent).toContain('Pedido não encontrado ou vencido.');
    expect(view.dialog().querySelector('input')).toBeTruthy(); // stays on the code, to try another
  });
});
