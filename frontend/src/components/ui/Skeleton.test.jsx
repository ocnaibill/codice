import { afterEach, describe, expect, it } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { Skeleton } from './Skeleton';

let view;
afterEach(() => view.unmount());

describe('Skeleton, the shape of what is loading', () => {
  it('says it is busy, by its label, and shimmers', async () => {
    view = await mount(<Skeleton label="Carregando a lista" className="h-10" />);
    const el = document.body.querySelector('[role="status"]');
    expect(el.getAttribute('aria-label')).toBe('Carregando a lista');
    expect(el.getAttribute('aria-busy')).toBe('true');
    expect(el.className).toContain('animate-shimmer');
    expect(el.className).toContain('h-10');
  });
});
