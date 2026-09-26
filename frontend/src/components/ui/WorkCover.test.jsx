import { act } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { WorkCover } from './WorkCover';
import { authenticatedUrl } from '../../lib/api';

vi.mock('../../lib/api', () => ({
  authenticatedUrl: vi.fn((url) => {
    if (!url) throw new Error('An asset URL is required');
    return url;
  }),
}));
let view;
afterEach(() => {
  view.unmount();
  vi.clearAllMocks();
});

describe('work cover fallback', () => {
  it('renders an accessible placeholder for a work without a cover', async () => {
    view = await mount(
      <WorkCover
        item={{ title: 'Duna', author: 'Frank Herbert', coverUrl: null }}
      />
    );
    expect(authenticatedUrl).not.toHaveBeenCalled();
    expect(
      view.container.querySelector('[role="img"]').getAttribute('aria-label')
    ).toBe('Duna');
    expect(view.text()).toContain('Frank Herbert');
  });

  it('replaces a broken image with the title and author', async () => {
    view = await mount(
      <WorkCover
        item={{
          title: 'Duna',
          author: 'Frank Herbert',
          coverUrl: '/missing.jpg',
        }}
      />
    );
    await act(async () =>
      view.container.querySelector('img').dispatchEvent(new Event('error'))
    );
    expect(view.container.querySelector('img')).toBeNull();
    expect(view.text()).toContain('Duna');
  });
});
