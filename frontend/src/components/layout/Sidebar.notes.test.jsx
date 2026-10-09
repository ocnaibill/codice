import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mount } from '../../features/admin/testUtils';
import { Sidebar } from './Sidebar';
import { useGlobalStore } from '../../store/useGlobalStore';

let view;
const current = () => [...document.body.querySelectorAll('[aria-current="page"]')].map((b) => b.textContent.trim());
beforeEach(() => useGlobalStore.setState({ notesOpen: false, adminOpen: false, searchQuery: '', libraryView: 'all', categoryPageId: null }));
afterEach(() => view.unmount());

describe('the Anotações item of the menu (#13)', () => {
  it('opens the notes and closes the drawer on a phone', async () => {
    const onNavigate = vi.fn();
    view = await mount(<Sidebar onNavigate={onNavigate} />);
    await view.click(view.buttonMatching(/Anotações/));
    expect(useGlobalStore.getState().notesOpen).toBe(true);
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });

  it('is the current page while the notes are open, and the library view is not', async () => {
    view = await mount(<Sidebar />);
    expect(current()).toEqual(['Todas as obras']);
    useGlobalStore.getState().openNotes();
    view.unmount();
    view = await mount(<Sidebar />);
    expect(current()).toEqual(['Anotações']);
  });

  it('is not the current page while a search is on screen', async () => {
    useGlobalStore.setState({ notesOpen: true, searchQuery: 'duna' });
    view = await mount(<Sidebar />);
    expect(current()).toEqual([]);
  });

  it('has no shelf as the current page while the page of a category is open', async () => {
    useGlobalStore.setState({ categoryPageId: 7 });
    view = await mount(<Sidebar />);
    expect(current()).toEqual([]);
  });

  it('sits with the person’s own collection, before the administration', async () => {
    view = await mount(<Sidebar canAdmin />);
    const text = document.body.querySelector('nav').textContent;
    expect(text.indexOf('Favoritos')).toBeLessThan(text.indexOf('Anotações'));
    expect(text.indexOf('Anotações')).toBeLessThan(text.indexOf('Administração'));
  });
});
