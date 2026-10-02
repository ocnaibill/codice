import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { ConfirmDialog } from '../../features/admin/components/ConfirmDialog';

let view;
afterEach(() => view.unmount());

describe('ConfirmDialog, how it appears', () => {
  it('fades the backdrop in and lets the dialog rise, with the motion of the system', async () => {
    view = await mount(<ConfirmDialog title="Apagar?" message="x" choices={[{ label: 'Apagar', value: 'a' }]} onChoose={vi.fn()} onCancel={vi.fn()} />);
    expect(view.dialog().className).toContain('animate-pop-in');
    expect(view.dialog().parentElement.className).toContain('animate-fade-in');
  });
});
