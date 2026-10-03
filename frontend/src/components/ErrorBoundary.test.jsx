import React from 'react';
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { mount } from '../features/admin/testUtils';
import { ErrorBoundary } from './ErrorBoundary';

function Boom({ message = 'quebrou' }) {
  throw new Error(message);
}

let view;
beforeEach(() => vi.spyOn(console, 'error').mockImplementation(() => {}));
afterEach(() => {
  view?.unmount();
  vi.restoreAllMocks();
});

describe('ErrorBoundary', () => {
  it('shows what is inside while nothing breaks', async () => {
    view = await mount(<ErrorBoundary><p>tudo bem</p></ErrorBoundary>);
    expect(view.text()).toBe('tudo bem');
    expect(document.body.querySelector('[role="alert"]')).toBeNull();
  });

  it('says in Portuguese that something went wrong, and what', async () => {
    view = await mount(<ErrorBoundary where="o leitor"><Boom /></ErrorBoundary>);
    expect(document.body.querySelector('[role="alert"]')).not.toBeNull();
    expect(view.text()).toContain('Algo deu errado');
    expect(view.text()).toContain('Tivemos um problema ao mostrar o leitor.');
    expect(view.text()).not.toMatch(/Something|unexpected|Reload\b/);
  });

  it('says "esta tela" when it is not told what broke', async () => {
    view = await mount(<ErrorBoundary><Boom /></ErrorBoundary>);
    expect(view.text()).toContain('ao mostrar esta tela.');
  });

  it('has the technical message folded, and none when there is none', async () => {
    view = await mount(<ErrorBoundary><Boom message="Cannot read x" /></ErrorBoundary>);
    const details = document.body.querySelector('details');
    expect(details.open).toBe(false);
    expect(details.querySelector('summary').textContent).toBe('Detalhes técnicos');
    expect(details.querySelector('pre').textContent).toBe('Cannot read x');
    view.unmount();
    function NoMessage() { throw new Error(''); }
    view = await mount(<ErrorBoundary><NoMessage /></ErrorBoundary>);
    expect(document.body.querySelector('details')).toBeNull();
  });

  it('reloads when the button says so, and does it its own way when it is told', async () => {
    const onReload = vi.fn();
    view = await mount(<ErrorBoundary onReload={onReload}><Boom /></ErrorBoundary>);
    await view.click(view.button('Recarregar'));
    expect(onReload).toHaveBeenCalledTimes(1);
  });

  it('offers a way back only when it is given one', async () => {
    view = await mount(<ErrorBoundary><Boom /></ErrorBoundary>);
    expect(view.button('Voltar')).toBeUndefined();
    view.unmount();
    const onBack = vi.fn();
    view = await mount(<ErrorBoundary onBack={onBack}><Boom /></ErrorBoundary>);
    await view.click(view.button('Voltar'));
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  it('is drawn with the colors of the app and none that is loose', async () => {
    view = await mount(<ErrorBoundary><Boom /></ErrorBoundary>);
    const html = document.body.innerHTML;
    expect(html).not.toMatch(/zinc-|red-\d|blue-\d/);
    expect(html).toContain('bg-danger-soft');
    expect(html).toContain('bg-brand');
  });

  it('writes what it caught to the console, for whoever is looking', async () => {
    view = await mount(<ErrorBoundary><Boom message="x" /></ErrorBoundary>);
    expect(console.error).toHaveBeenCalledWith('ErrorBoundary caught:', expect.any(Error), expect.anything());
  });
});
