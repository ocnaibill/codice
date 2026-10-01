import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act } from 'react';

vi.mock('../../../lib/download', () => ({ fetchFile: vi.fn(), saveBlob: vi.fn() }));

import { fetchFile, saveBlob } from '../../../lib/download';
import { mount, flush } from '../../admin/testUtils';
import { ExportNotes } from './ExportNotes';

const file = (text, name = 'codice-anotacoes-20261002.md') => ({ blob: new Blob([text]), name });
let view;
let onClose;

async function open(props = {}) {
  onClose = vi.fn();
  view = await mount(<ExportNotes filters={{ q: '', kind: '', tag: '', work: null }} total={3} onClose={onClose} {...props} />);
  await flush();
}
const radio = (value) => document.body.querySelector(`input[name="export-format"][value="${value}"]`);
const preview = () => document.body.querySelector('pre')?.textContent;
const save = () => view.button('Salvar arquivo');
const key = (k) => act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: k })); });

beforeEach(() => {
  vi.clearAllMocks();
  fetchFile.mockResolvedValue(file('# Anotações\n\n> Fear is the mind-killer.'));
});
afterEach(() => view.unmount());

describe('ExportNotes: the export, reviewed before it is saved (FL-09)', () => {
  it('makes the file with the filters, shows it and says what it is', async () => {
    await open({ filters: { q: ' areia ', kind: 'highlight', tag: 'ideia', work: { id: 7, title: 'Duna' } }, total: 1 });
    expect(fetchFile).toHaveBeenCalledTimes(1);
    expect(fetchFile).toHaveBeenCalledWith('/notes/export?q=areia&kind=highlight&tag=ideia&workId=7&format=md', 'codice-anotacoes.md');
    const text = view.text();
    expect(text).toContain('1 anotação: obra “Duna”, só destaques, tag #ideia, texto “areia”.');
    expect(preview()).toBe('# Anotações\n\n> Fear is the mind-killer.');
    expect(text).toContain('codice-anotacoes-20261002.md, 1 KB');
  });

  it('says how many in the plural, and all of them when nothing narrows it', async () => {
    await open({ total: 3 });
    expect(view.text()).toContain('3 anotações: todas as suas anotações.');
    expect(fetchFile.mock.calls[0][0]).toBe('/notes/export?format=md');
  });

  it('cannot be saved before the file is ready, and saves exactly the file that was shown, without asking again', async () => {
    let finish;
    fetchFile.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    await open();
    expect(view.text()).toContain('Preparando o arquivo…');
    expect(save().disabled).toBe(true);
    const ready = file('texto revisado', 'a.md');
    await act(async () => { finish(ready); });
    await flush();
    expect(save().disabled).toBe(false);
    await view.click(save());
    expect(saveBlob).toHaveBeenCalledTimes(1);
    expect(saveBlob).toHaveBeenCalledWith(ready.blob, 'a.md');
    expect(fetchFile).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalled();
  });

  it('makes the file again in the other format, and only the last one counts', async () => {
    let first;
    fetchFile.mockReturnValueOnce(new Promise((resolve) => { first = resolve; }));
    await open();
    fetchFile.mockResolvedValueOnce(file('{"count":3}', 'a.json'));
    await view.click(radio('json'));
    expect(fetchFile).toHaveBeenLastCalledWith('/notes/export?format=json', 'codice-anotacoes.json');
    expect(radio('json').checked).toBe(true);
    expect(preview()).toBe('{"count":3}');
    await act(async () => { first(file('velho', 'velho.md')); }); // the first answer arrives late
    await flush();
    expect(preview()).toBe('{"count":3}');
    await view.click(save());
    expect(saveBlob.mock.calls[0][1]).toBe('a.json');
  });

  it('shows the start of a long file and says there is more, and a short one whole', async () => {
    fetchFile.mockResolvedValue(file('x'.repeat(5000)));
    await open();
    expect(preview()).toBe(`${'x'.repeat(1800)}\n…`);
    view.unmount();
    fetchFile.mockResolvedValue(file('y'.repeat(1800)));
    await open();
    expect(preview()).toBe('y'.repeat(1800));
  });

  it('says so and does not offer to save when the file could not be made', async () => {
    fetchFile.mockRejectedValue(new Error('offline'));
    await open();
    expect(view.text()).toContain('Não foi possível preparar o arquivo.');
    expect(save().disabled).toBe(true);
    expect(preview()).toBeUndefined();
  });

  it('warns that one file holds at most ten thousand, the oldest first, only when there are more', async () => {
    await open({ total: 10001 });
    expect(view.text()).toContain('no máximo 10.000 anotações, as mais antigas primeiro');
    view.unmount();
    await open({ total: 10000 });
    expect(view.text()).not.toContain('no máximo');
  });

  it('is left with Cancelar, the cross, Escape or a click outside, but not by a click inside', async () => {
    await open();
    await view.click(document.body.querySelector('pre'));
    expect(onClose).not.toHaveBeenCalled();
    await view.click(view.button('Cancelar'));
    await view.click(view.button('✕'));
    await key('Escape');
    await view.click(view.dialog());
    expect(onClose).toHaveBeenCalledTimes(4);
  });

  it('leaves the Escape to a dialog opened over it', async () => {
    await open();
    const over = document.createElement('div');
    over.setAttribute('role', 'dialog');
    document.body.appendChild(over);
    await key('Escape');
    expect(onClose).not.toHaveBeenCalled();
    over.remove();
    await key('Escape');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not make the file again when the list behind it reloads with the same filters', async () => {
    await open({ filters: { q: 'a', kind: '', tag: '', work: { id: 1, title: 'X' } } });
    expect(fetchFile).toHaveBeenCalledTimes(1);
  });
});
