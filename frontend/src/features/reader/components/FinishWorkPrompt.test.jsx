import React from 'react';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { act } from 'react';
import { mount } from '../../admin/testUtils';
import { FinishWorkPrompt } from './FinishWorkPrompt';

const others = [{ file: { format: 'pdf', percentComplete: 42.4 }, edition: { language: 'pt' } }];
let view;
afterEach(() => view?.unmount());
const props = (extra = {}) => ({ finished: 'Você leu a edição em EPUB até o fim.', others, busy: false, onFinish: vi.fn(), onKeep: vi.fn(), ...extra });
const dialog = () => document.body.querySelector('[role="dialog"]');

describe('FinishWorkPrompt', () => {
  it('asks whether the work is finished, says what was finished and which version is still being read', async () => {
    view = await mount(<FinishWorkPrompt {...props()} />);
    expect(dialog().getAttribute('aria-label')).toBe('Obra finalizada?');
    expect(dialog().getAttribute('aria-modal')).toBe('true');
    expect(view.text()).toContain('Você terminou esta versão');
    expect(view.text()).toContain('Você leu a edição em EPUB até o fim.');
    expect(view.text()).toContain('Você ainda está em PDF (Português), 42%');
  });

  it('names more than one other version', async () => {
    const two = [...others, { file: { format: 'epub', percentComplete: 0 }, edition: {} }];
    view = await mount(<FinishWorkPrompt {...props({ others: two })} />);
    expect(view.text()).toContain('PDF (Português), 42% e EPUB');
  });

  it('finishes the work on "Sim", and leaves the other version on "Não"', async () => {
    const p = props();
    view = await mount(<FinishWorkPrompt {...p} />);
    await view.click(view.button('Sim, a obra está finalizada'));
    expect(p.onFinish).toHaveBeenCalledTimes(1);
    expect(p.onKeep).not.toHaveBeenCalled();
    await view.click(view.button('Não, continuar a outra versão depois'));
    expect(p.onKeep).toHaveBeenCalledTimes(1);
  });

  it('takes Escape for "Não": nothing is marked finished by leaving', async () => {
    const p = props();
    view = await mount(<FinishWorkPrompt {...p} />);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(p.onKeep).toHaveBeenCalledTimes(1);
    expect(p.onFinish).not.toHaveBeenCalled();
    for (const key of ['Enter', 'x', ' ', 'Tab']) {
      await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key })); });
    }
    expect(p.onKeep).toHaveBeenCalledTimes(1);
  });

  it('keeps listening for Escape with the answer it has now, when it is given another', async () => {
    const first = props();
    view = await mount(<FinishWorkPrompt {...first} />);
    const second = props();
    view.unmount();
    view = await mount(<FinishWorkPrompt {...second} />);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(second.onKeep).toHaveBeenCalledTimes(1);
    expect(first.onKeep).not.toHaveBeenCalled();
  });

  it('does not listen to the keyboard once it is gone', async () => {
    const p = props();
    view = await mount(<FinishWorkPrompt {...p} />);
    view.unmount();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(p.onKeep).not.toHaveBeenCalled();
    view = await mount(<div />);
  });

  it('does not let either answer be given twice while one is being saved', async () => {
    view = await mount(<FinishWorkPrompt {...props({ busy: true })} />);
    expect(view.button('Sim, a obra está finalizada').disabled).toBe(true);
    expect(view.button('Não, continuar a outra versão depois').disabled).toBe(true);
  });

  it('is drawn in the colors of the app, with buttons the thumb can hit', async () => {
    view = await mount(<FinishWorkPrompt {...props()} />);
    expect(dialog().innerHTML).not.toMatch(/zinc-|blue-\d|red-\d/);
    expect(view.button('Sim, a obra está finalizada').className).toContain('bg-brand');
    expect(view.button('Sim, a obra está finalizada').className).toContain('min-h-11');
    expect(view.button('Não, continuar a outra versão depois').className).toContain('min-h-11');
  });
});
