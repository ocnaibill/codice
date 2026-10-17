import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { PairingSwitch } from './PairingSwitch';

let view;
afterEach(() => view.unmount());
beforeEach(() => vi.clearAllMocks());

describe('PairingSwitch', () => {
  it('shows the state and changes it', async () => {
    api.get.mockResolvedValue({ data: { enabled: true } });
    api.put.mockResolvedValue({ data: { enabled: false } });
    view = await mount(<PairingSwitch />);
    const box = view.container.querySelector('input[type="checkbox"]');
    expect(box.checked).toBe(true);
    await view.click(box);
    expect(api.put).toHaveBeenCalledWith('/admin/device-pairing', { enabled: false });
  });
});
