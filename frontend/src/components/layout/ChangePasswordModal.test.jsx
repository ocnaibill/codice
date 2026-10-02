import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../lib/api', () => ({ api: { post: vi.fn() } }));

import { mount } from '../../features/admin/testUtils';
import { ChangePasswordModal } from './ChangePasswordModal';

let view;
afterEach(() => view.unmount());

describe('ChangePasswordModal, how it appears', () => {
  it('fades the backdrop in and lets the dialog rise, with the motion of the system', async () => {
    view = await mount(<ChangePasswordModal onClose={vi.fn()} />);
    expect(view.dialog().className).toContain('animate-pop-in');
    expect(view.dialog().parentElement.className).toContain('animate-fade-in');
  });
});
