import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react';
import { mount } from '../../features/admin/testUtils';
import { EXIT_MS, ToastRegion } from './ToastRegion';
import { toast, useToasts } from './toast';

let view;
const advance = (ms) => act(async () => { vi.advanceTimersByTime(ms); });
const shown = () => [...document.body.querySelectorAll('[role="status"], [role="alert"]')];
beforeEach(async () => {
  useToasts.getState().clear();
  view = await mount(<ToastRegion />);
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  view.unmount();
});

describe('the region of notices', () => {
  it('shows the title and the message, and says it is where the notices are', async () => {
    await act(async () => { toast.success('Metadados atualizados', { message: 'Duna' }); });
    expect(view.text()).toContain('Metadados atualizados');
    expect(view.text()).toContain('Duna');
    expect(document.body.querySelector('[aria-label="Avisos"]')).not.toBeNull();
  });

  it('announces an error as an alert and the rest as a status', async () => {
    await act(async () => { toast.error('Falhou'); toast.success('Salvo'); toast.info('Olha'); });
    const roles = shown().map((el) => el.getAttribute('role'));
    expect(roles).toEqual(['alert', 'status', 'status']);
  });

  it('comes in with a rise and leaves by itself after its time, going out with an animation first', async () => {
    await act(async () => { toast.success('Salvo'); });
    const el = shown()[0];
    expect(el.className).toContain('animate-rise-in');
    await advance(4999);
    expect(shown()).toHaveLength(1);
    expect(shown()[0].className).not.toContain('animate-toast-out');
    await advance(1);
    expect(shown()[0].className).toContain('animate-toast-out');
    expect(shown()).toHaveLength(1);
    await advance(EXIT_MS);
    expect(shown()).toHaveLength(0);
  });

  it('closes by its own button, and the action does what it says and closes it too', async () => {
    const onClick = vi.fn();
    await act(async () => { toast.info('Uma', { action: { label: 'Ver obra', onClick } }); toast.info('Outra'); });
    await act(async () => { view.button('Ver obra').click(); });
    expect(onClick).toHaveBeenCalledTimes(1);
    await advance(EXIT_MS);
    expect(view.text()).not.toContain('Uma');
    expect(view.text()).toContain('Outra');
    await act(async () => { document.querySelector('[aria-label="Fechar aviso"]').click(); });
    await advance(EXIT_MS);
    expect(shown()).toHaveLength(0);
  });

  it('has no button of action when there is none', async () => {
    await act(async () => { toast.info('Uma'); });
    expect(document.body.querySelectorAll('button')).toHaveLength(1);
  });

  it('stays while the pointer is on it, and goes on from where it was when the pointer leaves', async () => {
    await act(async () => { toast.success('Salvo'); });
    const el = shown()[0];
    await advance(3000);
    // React listens to mouseover/mouseout for enter and leave.
    await act(async () => { el.dispatchEvent(new MouseEvent('mouseover', { bubbles: true, relatedTarget: document.body })); });
    await advance(60000);
    expect(shown()).toHaveLength(1);
    expect(shown()[0].className).not.toContain('animate-toast-out');
    await act(async () => { el.dispatchEvent(new MouseEvent('mouseout', { bubbles: true, relatedTarget: document.body })); });
    await advance(1999);
    expect(shown()[0].className).not.toContain('animate-toast-out');
    await advance(1);
    expect(shown()[0].className).toContain('animate-toast-out');
  });

  it('gives at least a second more when the pointer comes at the last moment, so that it can be read', async () => {
    await act(async () => { toast.success('Salvo'); });
    const el = shown()[0];
    await advance(4800);
    await act(async () => { el.dispatchEvent(new MouseEvent('mouseover', { bubbles: true, relatedTarget: document.body })); });
    await advance(10000);
    await act(async () => { el.dispatchEvent(new MouseEvent('mouseout', { bubbles: true, relatedTarget: document.body })); });
    await advance(999);
    expect(shown()[0].className).not.toContain('animate-toast-out');
    await advance(1);
    expect(shown()[0].className).toContain('animate-toast-out');
  });

  it('stays while focus is inside it', async () => {
    await act(async () => { toast.info('Uma', { action: { label: 'Ver', onClick: () => {} } }); });
    const button = view.button('Ver');
    await act(async () => { button.focus(); });
    await advance(60000);
    expect(shown()).toHaveLength(1);
    await act(async () => { button.blur(); });
    await advance(5000 + EXIT_MS);
    expect(shown()).toHaveLength(0);
  });

  it('shows the three at most, the oldest leaving as a new one comes', async () => {
    await act(async () => { for (let i = 1; i <= 5; i += 1) toast.info(`n${i}`); });
    expect(view.text()).toContain('n5');
    expect(view.text()).not.toContain('n1');
    expect(shown()).toHaveLength(3);
  });

  it('replaces a notice that has the same key instead of piling up', async () => {
    await act(async () => { toast.success('Pronta', { key: 'work-1' }); });
    await act(async () => { toast.error('Falhou', { key: 'work-1' }); });
    expect(shown()).toHaveLength(1);
    expect(view.text()).toContain('Falhou');
  });

  it('leaves no timer running for a notice that is gone', async () => {
    await act(async () => { toast.success('Salvo'); });
    expect(vi.getTimerCount()).toBeGreaterThan(0);
    await act(async () => { useToasts.getState().clear(); });
    expect(vi.getTimerCount()).toBe(0);
  });

  it('does not let go of a notice that was dismissed twice', async () => {
    await act(async () => { toast.info('Uma'); });
    await act(async () => { document.querySelector('[aria-label="Fechar aviso"]').click(); });
    await advance(EXIT_MS + 5000);
    expect(shown()).toHaveLength(0);
  });
});
