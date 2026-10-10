import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { mount } from '../../admin/testUtils';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Shelf } from './Shelf';

vi.mock('../../../lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() }, authenticatedUrl: (u) => u }));

let view;
afterEach(() => view?.unmount());
beforeEach(() => {
  useGlobalStore.setState({ sheetWorkId: null, activeBookId: null, collectionSheetId: null });
});

const work = (id, title, over = {}) => ({ id, title, author: 'Autor', authors: [], coverUrl: '/c.jpg', tags: [], format: 'epub', mediaStatus: 'READY', fileCount: 1, fileUrl: '/f', fileId: id, ...over });

describe('Shelf', () => {
  it('has a name, how many there are, and the works one after the other', async () => {
    view = await mount(<Shelf headingId="h" title="Ficção científica" total={1420} items={[work(1, 'Duna'), work(2, 'Neuromancer')]} />);
    const section = document.body.querySelector('section');
    expect(section.getAttribute('aria-labelledby')).toBe('h');
    expect(document.body.querySelector('#h').textContent).toBe('Ficção científica');
    expect(section.textContent).toContain('[ 1.420 obras ]');
    expect([...document.body.querySelectorAll('li h3')].map((h) => h.textContent)).toEqual(['Duna', 'Neuromancer']);
    expect(document.body.querySelector('ul[aria-label="Ficção científica"]')).not.toBeNull();
  });

  it('says one in the singular: "[ 1 obra ]", and the other counts as they were', async () => {
    view = await mount(<Shelf title="Romance" total={1} items={[work(1, 'Duna')]} />);
    expect(document.body.textContent).toContain('[ 1 obra ]');
    expect(document.body.textContent).not.toContain('1 obras');
    view.unmount();
    view = await mount(<Shelf title="Romance" total={1} countWord="itens" items={[work(1, 'Duna')]} />);
    expect(document.body.textContent).toContain('[ 1 item ]');
    view.unmount();
    view = await mount(<Shelf title="Romance" total={0} items={[]} />);
    expect(document.body.textContent).toContain('[ 0 obras ]');
  });

  it('counts by the word it is given, and has no count when it is not told one', async () => {
    view = await mount(<Shelf title="A" total={5} countWord="itens" items={[work(1, 'Duna')]} />);
    expect(document.body.textContent).toContain('[ 5 itens ]');
    view.unmount();
    view = await mount(<Shelf title="A" items={[work(1, 'Duna')]} />);
    expect(document.body.textContent).not.toContain('[');
  });

  it('opens the sheet of a work from its cover and a series from its card, and goes on with what is read', async () => {
    const series = work(7, 'One Piece 121', { collapsed: { collectionId: 33, name: 'One Piece', newCount: 0, continue: null, total: 12, read: 12 } });
    view = await mount(<Shelf title="Mangás" items={[work(1, 'Duna'), series]} />);
    await view.click(document.body.querySelector('button.library-book-cover[title="Ver edições e arquivos"]'));
    expect(useGlobalStore.getState().sheetWorkId).toBe(1);
    await view.click(document.body.querySelector('[aria-label="Abrir a série One Piece"]'));
    expect(useGlobalStore.getState().collectionSheetId).toBe(33);
  });

  it('says, while it waits, that it is waiting, and does not hold the works it does not have', async () => {
    view = await mount(<Shelf title="A" items={[]} isLoading />);
    expect(document.body.querySelector('section').getAttribute('aria-busy')).toBe('true');
    expect(document.body.querySelectorAll('.library-carousel-item')).toHaveLength(5);
    expect(document.body.querySelector('li h3')).toBeNull();
  });

  it('says what there is when there is nothing', async () => {
    view = await mount(<Shelf title="A" items={[]} emptyText="Nada aqui ainda." />);
    expect(document.body.textContent).toContain('Nada aqui ainda.');
    view.unmount();
    view = await mount(<Shelf title="A" items={[]} />);
    expect(document.body.textContent).toContain('Nenhuma obra encontrada nessa categoria ainda.');
  });

  it('has a link to the rest when it is given one', async () => {
    const open = vi.fn();
    view = await mount(<Shelf title="A" items={[work(1, 'Duna')]} seeAll={{ label: 'Ver categoria', onClick: open }} />);
    await view.click(view.buttonMatching(/Ver categoria/));
    expect(open).toHaveBeenCalledTimes(1);
    view.unmount();
    view = await mount(<Shelf title="A" items={[work(1, 'Duna')]} />);
    expect(view.buttonMatching(/Ver categoria/)).toBeUndefined();
  });
});
