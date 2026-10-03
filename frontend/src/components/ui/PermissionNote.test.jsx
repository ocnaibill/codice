import React from 'react';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { NoPermission, PermissionNote } from './PermissionNote';
import { LoadError } from './LoadError';

let view;
afterEach(() => view?.unmount());

describe('PermissionNote', () => {
  it('says what is not given, as a note and not as an error', async () => {
    view = await mount(<PermissionNote>Só o dono do acervo liga o OCR.</PermissionNote>);
    const note = document.body.querySelector('[role="note"]');
    expect(note.textContent).toBe('Só o dono do acervo liga o OCR.');
    expect(document.body.querySelector('[role="alert"]')).toBeNull();
  });

  it('has a lock that a screen reader leaves out, and takes the place it is given', async () => {
    view = await mount(<PermissionNote className="mt-3">x</PermissionNote>);
    const note = document.body.querySelector('[role="note"]');
    expect(note.querySelector('svg').getAttribute('aria-hidden')).toBe('true');
    expect(note.className).toContain('mt-3');
  });
});

describe('NoPermission', () => {
  it('says the person is not allowed in, and what to do', async () => {
    view = await mount(<NoPermission />);
    const box = document.body.querySelector('[role="alert"]');
    expect(box.textContent).toContain('Você não tem permissão para ver isto');
    expect(box.textContent).toContain('Fale com quem cuida do acervo');
    expect(document.body.querySelector('button')).toBeNull();
  });

  it('says what it is told, and offers the way out it is given', async () => {
    const onBack = vi.fn();
    view = await mount(<NoPermission title="É de quem administra" onBack={onBack} backLabel="Sair daqui">Sua conta é de leitura.</NoPermission>);
    expect(document.body.textContent).toContain('É de quem administra');
    expect(document.body.textContent).toContain('Sua conta é de leitura.');
    await view.click(view.button('Sair daqui'));
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  it('is "Voltar ao acervo" by default', async () => {
    view = await mount(<NoPermission onBack={vi.fn()} />);
    expect(view.button('Voltar ao acervo')).toBeDefined();
  });
});

describe('LoadError when the server said no', () => {
  const refused = (status) => ({ response: { status } });

  it('says the person has no permission, and does not offer to try again, which would be the same no', async () => {
    view = await mount(<LoadError onRetry={vi.fn()} error={refused(403)}>Não foi possível carregar as contas.</LoadError>);
    expect(document.body.querySelector('[role="note"]').textContent).toContain('Você não tem permissão para ver isto');
    expect(view.button('Tentar de novo')).toBeUndefined();
    expect(document.body.textContent).not.toContain('Não foi possível carregar as contas.');
  });

  it('asks to try again for every other failure', async () => {
    for (const error of [refused(500), refused(502), refused(404), refused(401), new Error('x'), undefined]) {
      view = await mount(<LoadError onRetry={vi.fn()} error={error}>Não foi possível carregar.</LoadError>);
      expect(view.button('Tentar de novo'), String(error?.response?.status)).toBeDefined();
      view.unmount();
    }
    view = await mount(<div />);
  });
});
