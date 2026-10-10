import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({ api: { put: vi.fn(), delete: vi.fn() } }));

import { api } from '../../../lib/api';
import { WorkRating } from './WorkRating';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container;
let root;
let client;
const render = async (rating) => {
  await act(async () => { root.render(<QueryClientProvider client={client}><WorkRating workId={7} rating={rating} /></QueryClientProvider>); });
};
const stars = () => [...container.querySelectorAll('button')];

beforeEach(() => {
  vi.clearAllMocks();
  api.put.mockResolvedValue({});
  api.delete.mockResolvedValue({});
  client = new QueryClient();
  vi.spyOn(client, 'invalidateQueries');
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe('WorkRating', () => {
  it('has five stars, filled up to the rating, and says it in words', async () => {
    await render(3);
    expect(stars()).toHaveLength(5);
    expect(stars().map((b) => b.textContent)).toEqual(['★', '★', '★', '☆', '☆']);
    expect(stars().map((b) => b.getAttribute('aria-pressed'))).toEqual(['false', 'false', 'true', 'false', 'false']);
    expect(container.textContent).toContain('3 de 5');
    expect(stars()[2].getAttribute('aria-label')).toBe('Tirar sua nota de 3 estrelas');
    expect(stars()[0].getAttribute('aria-label')).toBe('Dar 1 estrela');
  });

  it('says nothing of a number when there is no rating', async () => {
    await render(0);
    expect(stars().map((b) => b.textContent)).toEqual(['☆', '☆', '☆', '☆', '☆']);
    expect(container.textContent).not.toContain('de 5');
  });

  it('gives the stars of the one pressed, and asks the page again', async () => {
    await render(0);
    await act(async () => { stars()[3].click(); });
    expect(api.put).toHaveBeenCalledWith('/works/7/rating', { stars: 4 });
    expect(client.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['work', 7] });
  });

  it('changes it with another star, and takes it away with the one that was given', async () => {
    await render(4);
    await act(async () => { stars()[1].click(); });
    expect(api.put).toHaveBeenCalledWith('/works/7/rating', { stars: 2 });
    await act(async () => { stars()[3].click(); });
    expect(api.delete).toHaveBeenCalledWith('/works/7/rating');
  });
});
