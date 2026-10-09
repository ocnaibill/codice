import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import axe from 'axe-core';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { PdfPassword } from './PdfPassword';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let onSubmit;
let onCancel;
const field = () => container.querySelector('input[type="password"]');
const button = (text) => [...container.querySelectorAll('button')].find((b) => b.textContent === text);
const type = (value) => act(async () => {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field(), value);
  field().dispatchEvent(new Event('input', { bubbles: true }));
});
const show = async (props = {}) => {
  await act(async () => { root.render(<PdfPassword wrong={false} onSubmit={onSubmit} onCancel={onCancel} {...props} />); });
};

beforeEach(() => {
  onSubmit = vi.fn();
  onCancel = vi.fn();
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('PdfPassword (#89)', () => {
  it('says the PDF has a password, and that it is not kept', async () => {
    await show();
    const dialog = container.querySelector('[role="dialog"]');
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.getAttribute('aria-label')).toBe('Este PDF tem senha');
    expect(container.querySelector('h2').textContent).toBe('Este PDF tem senha');
    expect(container.textContent).toContain('Ela não é guardada: vale só para esta leitura.');
  });

  it('has a password field the browser does not fill or remember, with the focus on it', async () => {
    await show();
    expect(field().autocomplete).toBe('off');
    expect(field().labels[0].textContent).toContain('Senha do PDF');
    expect(document.activeElement).toBe(field());
  });

  it('sends what was typed when the person opens, and not before', async () => {
    await show();
    expect(button('Abrir').disabled).toBe(true);
    await type('segredo');
    expect(button('Abrir').disabled).toBe(false);
    expect(onSubmit).not.toHaveBeenCalled();
    await act(async () => { button('Abrir').click(); });
    expect(onSubmit).toHaveBeenCalledWith('segredo');
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('sends it with Enter too, and sends nothing when the field is empty, not even with the form', async () => {
    await show();
    await act(async () => { container.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(onSubmit).not.toHaveBeenCalled();
    await type('abc');
    await act(async () => { container.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    expect(onSubmit).toHaveBeenCalledWith('abc');
  });

  it('keeps the spaces of the password as they were typed', async () => {
    await show();
    await type('  com espaços  ');
    await act(async () => { button('Abrir').click(); });
    expect(onSubmit).toHaveBeenCalledWith('  com espaços  ');
  });

  it('says when the last password was wrong, linked to the field, and says nothing before', async () => {
    await show();
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(field().getAttribute('aria-invalid')).toBeNull();
    await show({ wrong: true });
    const alert = container.querySelector('[role="alert"]');
    expect(alert.textContent).toBe('Senha incorreta. Tente de novo.');
    expect(field().getAttribute('aria-invalid')).toBe('true');
    expect(field().getAttribute('aria-describedby')).toBe(alert.id);
  });

  it('gives up on Cancelar and on Escape', async () => {
    await show();
    await act(async () => { button('Cancelar').click(); });
    expect(onCancel).toHaveBeenCalledTimes(1);
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    expect(onCancel).toHaveBeenCalledTimes(2);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('has no accessibility violation (axe), asking and after a wrong answer', async () => {
    const options = {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
      rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
    };
    await show();
    expect((await axe.run(document.body, options)).violations.map((v) => v.id)).toEqual([]);
    await show({ wrong: true });
    expect((await axe.run(document.body, options)).violations.map((v) => v.id)).toEqual([]);
  });
});
