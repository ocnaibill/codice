import { describe, it, expect, afterEach } from 'vitest';
import { mount } from '../../admin/testUtils';
import { GreetingStats } from './GreetingStats';

let view;
afterEach(() => view.unmount());

describe('GreetingStats', () => {
  it('greets the signed-in account by name', async () => {
    view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
    expect(view.text()).toContain('ana :)');
    expect(view.text()).not.toContain('Bianco');
  });

  it('greets without a name while the account is still loading', async () => {
    view = await mount(<GreetingStats userName={undefined} stats={null} isLoading />);
    expect(view.text()).not.toContain('undefined');
    expect(view.text()).not.toContain(':)');
  });
});
