import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

import { EquivalentPositionPrompt } from './EquivalentPositionPrompt';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const from = { format: 'pdf', language: 'pt', percentComplete: 42 };
const found = [{ method: 'text', confidence: 'high', section: 'Capítulo 3', excerpt: 'Um trecho encontrado.', locator: { type: 'epub', href: 'c3.xhtml' } }];
const ambiguous = [
  { method: 'text', confidence: 'medium', section: 'Nota', excerpt: 'Primeira opção.', locator: { type: 'epub', href: 'a.xhtml' } },
  { method: 'anchors', confidence: 'medium', section: 'Outra nota', excerpt: 'Segunda opção.', locator: { type: 'epub', href: 'b.xhtml' } },
];

let container;
let root;
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent.trim() === text);

async function render(props) {
  await act(async () => {
    root.render(
      <EquivalentPositionPrompt from={from} sourceExcerpt="Um trecho de origem." status="found" candidates={found} busy={false} onAccept={vi.fn()} onDecline={vi.fn()} {...props} />
    );
  });
}

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('EquivalentPositionPrompt', () => {
  it('says where the person was and shows the one candidate found', async () => {
    await render();
    expect(container.textContent).toContain('Você está em 42% no PDF (Português)');
    expect(container.textContent).toContain('Um trecho de origem.');
    expect(container.textContent).toContain('mesmo trecho, alta confiança');
    expect(container.textContent).toContain('Capítulo 3');
    expect(container.textContent).toContain('Um trecho encontrado.');
    expect(container.querySelectorAll('input[type=radio]').length).toBe(0);
  });

  it('accepting calls onAccept with the found candidate', async () => {
    const onAccept = vi.fn();
    await render({ onAccept });
    await act(async () => { button('Continuar daqui').click(); });
    expect(onAccept).toHaveBeenCalledWith(found[0]);
  });

  it('declining calls onDecline and nothing else', async () => {
    const onAccept = vi.fn();
    const onDecline = vi.fn();
    await render({ onAccept, onDecline });
    await act(async () => { button('Não, abrir minha posição').click(); });
    expect(onDecline).toHaveBeenCalledTimes(1);
    expect(onAccept).not.toHaveBeenCalled();
  });

  it('lists every candidate when ambiguous, and accepts the one chosen', async () => {
    const onAccept = vi.fn();
    await render({ status: 'ambiguous', candidates: ambiguous, onAccept });
    expect(container.querySelectorAll('input[type=radio]').length).toBe(2);
    expect(container.textContent).toContain('Primeira opção.');
    expect(container.textContent).toContain('Segunda opção.');

    // The first is chosen by default.
    await act(async () => { button('Continuar daqui').click(); });
    expect(onAccept).toHaveBeenLastCalledWith(ambiguous[0]);

    // Choosing the second one accepts that one instead.
    const radios = [...container.querySelectorAll('input[type=radio]')];
    await act(async () => { radios[1].click(); });
    await act(async () => { button('Continuar daqui').click(); });
    expect(onAccept).toHaveBeenLastCalledWith(ambiguous[1]);
  });

  it('disables its buttons while busy', async () => {
    await render({ busy: true });
    expect(button('Continuar daqui').disabled).toBe(true);
    expect(button('Não, abrir minha posição').disabled).toBe(true);
  });
});

describe('EquivalentPositionPrompt: in the system of the app', () => {
  it('is a modal dialog drawn in the colors of the app, with buttons the thumb can hit', async () => {
    await render();
    const dialog = container.querySelector('[role="dialog"]');
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.innerHTML).not.toMatch(/zinc-|blue-\d|red-\d/);
    expect(button('Continuar daqui').className).toContain('bg-brand');
    expect(button('Continuar daqui').className).toContain('min-h-11');
    expect(button('Não, abrir minha posição').className).toContain('min-h-11');
  });

  it('takes Escape for declining, and listens to nothing else, or once it is gone', async () => {
    const onDecline = vi.fn();
    const onAccept = vi.fn();
    await render({ onDecline, onAccept });
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(onDecline).toHaveBeenCalledTimes(1);
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' })); });
    expect(onDecline).toHaveBeenCalledTimes(1);
    expect(onAccept).not.toHaveBeenCalled();
    act(() => root.unmount());
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(onDecline).toHaveBeenCalledTimes(1);
    root = createRoot(container);
  });

  it('answers Escape with the function it was last given', async () => {
    const first = vi.fn();
    const second = vi.fn();
    await render({ onDecline: first });
    await render({ onDecline: second });
    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    expect(second).toHaveBeenCalledTimes(1);
    expect(first).not.toHaveBeenCalled();
  });

  it('marks the candidate that is chosen with the brand', async () => {
    await render({ status: 'ambiguous', candidates: ambiguous });
    const labels = [...container.querySelectorAll('label')];
    expect(labels.every((l) => l.className.includes('has-[:checked]:border-brand'))).toBe(true);
    expect(container.querySelector('input[type="radio"]').className).toContain('accent-brand');
  });
});
