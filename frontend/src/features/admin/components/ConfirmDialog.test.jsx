import { describe, it, expect, vi, afterEach } from 'vitest';
import { mount } from '../testUtils';
import { ConfirmDialog } from './ConfirmDialog';

let view;
afterEach(() => view.unmount());

describe('ConfirmDialog: a choice that is not available', () => {
  it('shows a choice marked disabled as disabled, and does not answer it', async () => {
    const onChoose = vi.fn();
    view = await mount(<ConfirmDialog title="Criar" message="x" choices={[{ label: 'Criar', value: 1, disabled: true }, { label: 'Outra', value: 2 }]} onChoose={onChoose} onCancel={() => {}} />);
    expect(view.button('Criar').disabled).toBe(true);
    expect(view.button('Outra').disabled).toBe(false);
    await view.click(view.button('Criar'));
    expect(onChoose).not.toHaveBeenCalled();
    await view.click(view.button('Outra'));
    expect(onChoose).toHaveBeenCalledWith(2);
  });
});
