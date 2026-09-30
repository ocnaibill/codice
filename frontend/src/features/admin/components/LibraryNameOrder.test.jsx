import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { LibraryNameOrder } from './LibraryNameOrder';

let view;
const radio = (label) => [...document.body.querySelectorAll('label')].find((l) => l.textContent.includes(label))?.querySelector('input');

async function open(library = 'given_first') {
  api.get.mockResolvedValue({ data: { choice: '', library, effective: library } });
  api.put.mockResolvedValue({ data: {} });
  view = await mount(<LibraryNameOrder />);
}
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('LibraryNameOrder', () => {
  it('shows the library default that applies and says each account can choose its own', async () => {
    await open('family_first');
    expect(radio('Sobrenome, Nome').checked).toBe(true);
    expect(radio('Nome Sobrenome').checked).toBe(false);
    expect(view.text()).toContain('Cada conta pode escolher o seu em Preferências');
  });

  it('saves the library default when the owner picks one', async () => {
    await open('given_first');
    await view.click(radio('Sobrenome, Nome'));
    expect(api.put).toHaveBeenCalledWith('/admin/name-order', { nameOrder: 'family_first' });
  });

  it('shows what the server said when it cannot save', async () => {
    await open();
    api.put.mockRejectedValue({ response: { status: 403, data: 'Forbidden\n' } });
    await view.click(radio('Sobrenome, Nome'));
    expect(view.text()).toContain('Forbidden');
  });
});
