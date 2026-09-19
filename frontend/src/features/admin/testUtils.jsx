import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

export const flush = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

/** Renders into a fresh container and returns helpers to find and click things. */
export async function mount(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  await act(async () => {
    root.render(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
  });
  await flush();
  const all = (selector) => [...document.body.querySelectorAll(selector)];
  return {
    container,
    text: () => document.body.textContent,
    button: (label) => all('button').find((b) => b.textContent.trim() === label),
    buttonMatching: (pattern) => all('button').find((b) => pattern.test(b.textContent)),
    click: async (target) => { await act(async () => { target.click(); }); await flush(); },
    type: async (input, value) => {
      await act(async () => {
        const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
        setter.call(input, value);
        input.dispatchEvent(new Event('input', { bubbles: true }));
      });
    },
    dialog: () => document.body.querySelector('[role="dialog"]'),
    unmount: () => { act(() => root.unmount()); container.remove(); },
  };
}
