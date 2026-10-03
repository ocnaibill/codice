import React from 'react';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { Notice } from './Notice';
import { LoadError } from './LoadError';

let view;
afterEach(() => view?.unmount());
const box = () => document.body.querySelector('[role="alert"], [role="status"]');

describe('Notice', () => {
  it('says what happened, with a title when there is one', async () => {
    view = await mount(<Notice tone="warning" title="Atenção">O arquivo mudou.</Notice>);
    expect(box().textContent).toContain('Atenção');
    expect(box().textContent).toContain('O arquivo mudou.');
    expect(box().querySelector('p').className).toContain('text-warning');
  });

  it('is an alert for an error and a status for the others', async () => {
    view = await mount(<div><Notice tone="danger">a</Notice><Notice tone="warning">b</Notice><Notice tone="success">c</Notice><Notice tone="info">d</Notice></div>);
    expect([...document.body.querySelectorAll('[role="alert"], [role="status"]')].map((n) => n.getAttribute('role'))).toEqual(['alert', 'status', 'status', 'status']);
  });

  it('takes the colors of its tone, and the info tone for one it does not know', async () => {
    view = await mount(<div><Notice tone="danger">a</Notice><Notice tone="success">b</Notice><Notice tone="nope">c</Notice></div>);
    const boxes = [...document.body.querySelectorAll('[role="alert"], [role="status"]')];
    expect(boxes[0].className).toContain('border-danger/30');
    expect(boxes[0].className).toContain('bg-danger-soft/40');
    expect(boxes[1].className).toContain('border-success/30');
    expect(boxes[2].className).toContain('bg-surface-alt/70');
  });

  it('has the action beside the text, and none when it has none', async () => {
    view = await mount(<Notice tone="info" action={<button>Fazer</button>}>texto</Notice>);
    expect(view.button('Fazer')).toBeDefined();
    view.unmount();
    view = await mount(<Notice>texto</Notice>);
    expect(document.body.querySelector('button')).toBeNull();
  });

  it('hides its icon from a screen reader', async () => {
    view = await mount(<Notice>texto</Notice>);
    expect(box().querySelector('[aria-hidden="true"]')).not.toBeNull();
  });
});

describe('LoadError', () => {
  it('says it could not load, and what it was, with a button to ask again', async () => {
    view = await mount(<LoadError onRetry={vi.fn()}>Não foi possível carregar as contas.</LoadError>);
    expect(box().getAttribute('role')).toBe('alert');
    expect(box().textContent).toContain('Não foi possível carregar as contas.');
    expect(view.button('Tentar de novo')).toBeDefined();
  });

  it('asks again when the button is pressed', async () => {
    const onRetry = vi.fn();
    view = await mount(<LoadError onRetry={onRetry}>x</LoadError>);
    await view.click(view.button('Tentar de novo'));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('says it is trying again, and does not let it be pressed while it is', async () => {
    const onRetry = vi.fn();
    view = await mount(<LoadError onRetry={onRetry} retrying>x</LoadError>);
    const button = view.button('Tentando…');
    expect(button.disabled).toBe(true);
    expect(view.button('Tentar de novo')).toBeUndefined();
  });

  it('has a message of its own when it is given none', async () => {
    view = await mount(<LoadError onRetry={vi.fn()} />);
    expect(box().textContent).toContain('Não foi possível carregar.');
  });
});
