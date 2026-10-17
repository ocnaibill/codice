import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../../admin/testUtils';
import { SendToKindle } from './SendToKindle';

let view;
async function open(kindle, file = { id: 7, format: 'epub' }) {
  api.get.mockResolvedValue({ data: kindle });
  view = await mount(<SendToKindle workId={3} file={file} />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('SendToKindle', () => {
  it('is not there when the server does not send books by e-mail', async () => {
    await open({ email: 'a@kindle.com', available: false });
    expect(view.button('Enviar para o Kindle')).toBeUndefined();
  });

  it('is not there for a format a Kindle does not take', async () => {
    await open({ email: 'a@kindle.com', available: true }, { id: 8, format: 'cbz' });
    expect(view.button('Enviar para o Kindle')).toBeUndefined();
  });

  it('points to the preferences when the account has no address', async () => {
    await open({ email: '', available: true });
    const button = view.button('Enviar para o Kindle');
    expect(button.disabled).toBe(true);
    expect(button.title).toContain('Preferências');
  });

  it('sends the file and says where it went', async () => {
    api.post.mockResolvedValue({ data: { sent: true, to: 'a@kindle.com' } });
    await open({ email: 'a@kindle.com', available: true });
    await view.click(view.button('Enviar para o Kindle'));
    expect(api.post).toHaveBeenCalledWith('/works/3/send-to-kindle', { fileId: 7 });
    expect(view.text()).toContain('Enviado para a@kindle.com');
  });

  it('says what the server said when it could not send', async () => {
    api.post.mockRejectedValue({ response: { status: 413, data: 'O arquivo tem 30 MB e o limite de envio é de 25 MB.' } });
    await open({ email: 'a@kindle.com', available: true });
    await view.click(view.button('Enviar para o Kindle'));
    expect(view.text()).toContain('limite de envio é de 25 MB');

    api.post.mockRejectedValue({ response: { status: 502, data: 'Não foi possível enviar: o servidor recusou o remetente' } });
    await view.click(view.button('Enviar para o Kindle'));
    expect(view.text()).toContain('recusou o remetente');

    api.post.mockRejectedValue({ response: { status: 500, data: 'Error sending' } });
    await view.click(view.button('Enviar para o Kindle'));
    expect(view.text()).toContain('Não foi possível enviar o livro.');
  });
});
