import { beforeEach, describe, expect, it } from 'vitest';
import { DURATIONS, MAX_TOASTS, toast, useToasts } from './toast';

const items = () => useToasts.getState().items;
beforeEach(() => useToasts.getState().clear());

describe('the notices on screen', () => {
  it('shows one, in the tone asked, with the time that tone has', () => {
    toast.success('Salvo', { message: 'Tudo certo' });
    toast.error('Falhou');
    toast.warning('Cuidado');
    toast.info('Olha');
    expect(items().map((t) => [t.tone, t.title, t.duration])).toEqual([
      ['success', 'Salvo', DURATIONS.success],
      ['error', 'Falhou', DURATIONS.error],
      ['warning', 'Cuidado', DURATIONS.warning],
      ['info', 'Olha', DURATIONS.info],
    ].slice(-MAX_TOASTS));
    expect(items()[0].tone).not.toBe('success');
  });

  it('keeps an error longer than a success: it has to be read', () => {
    expect(DURATIONS.error).toBeGreaterThan(DURATIONS.warning);
    expect(DURATIONS.warning).toBeGreaterThan(DURATIONS.success);
  });

  it('takes the time asked for, and the message and action as they come', () => {
    const action = { label: 'Ver', onClick: () => {} };
    toast.info('Olha', { duration: 1234, message: 'm', action });
    expect(items()[0]).toMatchObject({ duration: 1234, message: 'm', action });
  });

  it('gives each a number of its own, and dismisses by it', () => {
    const a = useToasts.getState().show({ title: 'a' });
    const b = useToasts.getState().show({ title: 'b' });
    expect(a).not.toBe(b);
    toast.dismiss(a);
    expect(items().map((t) => t.title)).toEqual(['b']);
    toast.dismiss(999);
    expect(items()).toHaveLength(1);
  });

  it('is an info notice when no tone is given', () => {
    useToasts.getState().show({ title: 'x' });
    expect(items()[0].tone).toBe('info');
  });

  it('puts a notice with the same key in the place of the one on screen', () => {
    toast.success('Pronta', { key: 'work-1' });
    toast.info('Outra');
    toast.error('Falhou', { key: 'work-1' });
    expect(items().map((t) => t.title)).toEqual(['Outra', 'Falhou']);
  });

  it('does not join notices that have no key', () => {
    toast.info('a');
    toast.info('a');
    expect(items()).toHaveLength(2);
  });

  it('pushes the oldest out beyond the most there can be', () => {
    for (let i = 1; i <= MAX_TOASTS + 2; i += 1) toast.info(`n${i}`);
    expect(items().map((t) => t.title)).toEqual(['n3', 'n4', 'n5']);
  });
});
