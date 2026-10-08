import { describe, it, expect, afterEach, vi } from 'vitest';
import axe from 'axe-core';
import { mount } from '../../admin/testUtils';
import { NextInSeriesPrompt } from './NextInSeriesPrompt';

let view;
afterEach(() => view.unmount());

const show = (over = {}) => {
  const props = { finished: 'One Piece Cap. 2', next: 'Cap. 3', onRead: vi.fn(), onDismiss: vi.fn(), ...over };
  return mount(<NextInSeriesPrompt {...props} />).then((v) => {
    view = v;
    return props;
  });
};

describe('NextInSeriesPrompt (#187)', () => {
  it('says what was finished and what comes next, as a status and not an alert', async () => {
    await show();
    expect(view.text()).toContain('Você terminou One Piece Cap. 2. O próximo da série é Cap. 3.');
    expect(document.body.querySelector('[role="status"]').getAttribute('aria-label')).toBe('Próximo da série');
    expect(document.body.querySelector('[role="alert"]')).toBeNull();
  });

  it('reads the next, or goes away, and nothing else', async () => {
    const props = await show();
    await view.click(view.button('Ler Cap. 3 agora'));
    expect(props.onRead).toHaveBeenCalledTimes(1);
    expect(props.onDismiss).not.toHaveBeenCalled();
    await view.click(view.button('Agora não'));
    expect(props.onDismiss).toHaveBeenCalledTimes(1);
    expect(props.onRead).toHaveBeenCalledTimes(1);
  });

  it('has no accessibility violation (axe)', async () => {
    await show();
    const result = await axe.run(document.body, { rules: { 'color-contrast': { enabled: false } } });
    expect(result.violations.map((v) => v.id)).toEqual([]);
  });
});
