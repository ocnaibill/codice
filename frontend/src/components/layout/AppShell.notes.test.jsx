import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { AppShell } from './AppShell';
import { useGlobalStore } from '../../store/useGlobalStore';

let view;
const bottom = () => document.body.querySelector('.library-bottom-nav');
const current = () => [...bottom().querySelectorAll('[aria-current="page"]')].map((b) => b.textContent.trim());
beforeEach(() => useGlobalStore.setState({ notesOpen: false, adminOpen: false, searchQuery: '', libraryView: 'all' }));
afterEach(() => view.unmount());

describe('the bottom bar of a phone (#13)', () => {
  it('has the notes, and opening them is the current page instead of the library', async () => {
    view = await mount(<AppShell searchQuery="" canAdmin={false}><p>x</p></AppShell>);
    expect(current()).toEqual(['Acervo']);
    const button = [...bottom().querySelectorAll('button')].find((b) => b.textContent.includes('Anotações'));
    await view.click(button);
    expect(useGlobalStore.getState().notesOpen).toBe(true);
    expect(current()).toEqual(['Anotações']);
  });

  it('does not mark the notes while a search is on screen', async () => {
    useGlobalStore.setState({ notesOpen: true });
    view = await mount(<AppShell searchQuery="duna" canAdmin={false}><p>x</p></AppShell>);
    expect(current()).toEqual(['Buscar']);
  });
});
