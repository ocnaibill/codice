import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../api', () => ({ api: { get: vi.fn() } }));

import { api } from '../api';
import { downloadFile, fetchFile, saveBlob } from '../download';

let clicked;
beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  clicked = [];
  URL.createObjectURL = vi.fn(() => 'blob:fake');
  URL.revokeObjectURL = vi.fn();
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function click() {
    clicked.push({ href: this.href, download: this.download, attached: document.body.contains(this) });
  });
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('fetchFile', () => {
  it('asks for the file as a blob with the session, and names it as the server did', async () => {
    const blob = new Blob(['x']);
    api.get.mockResolvedValue({ data: blob, headers: { 'content-disposition': 'attachment; filename="codice-anotacoes-20261002.md"' } });
    expect(await fetchFile('/notes/export?format=md', 'fallback.md')).toEqual({ blob, name: 'codice-anotacoes-20261002.md' });
    expect(api.get).toHaveBeenCalledWith('/notes/export?format=md', { responseType: 'blob', timeout: 60000 });
  });

  it('uses the fallback name when the server gave none', async () => {
    api.get.mockResolvedValue({ data: new Blob(['x']), headers: {} });
    expect((await fetchFile('/x', 'fallback.md')).name).toBe('fallback.md');
    api.get.mockResolvedValue({ data: new Blob(['x']) });
    expect((await fetchFile('/x', 'other.md')).name).toBe('other.md');
  });

  it('lets a failure through for the caller to say so', async () => {
    api.get.mockRejectedValue(new Error('offline'));
    await expect(fetchFile('/x', 'a.md')).rejects.toThrow('offline');
  });
});

describe('saveBlob', () => {
  it('hands the file to the browser as a download and lets go of it afterwards', () => {
    saveBlob(new Blob(['x']), 'notas.md');
    expect(clicked).toEqual([{ href: 'blob:fake', download: 'notas.md', attached: true }]);
    expect(document.body.querySelector('a[download]')).toBeNull();
    expect(URL.revokeObjectURL).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1000);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:fake');
  });
});

describe('downloadFile', () => {
  it('fetches and saves in one go', async () => {
    api.get.mockResolvedValue({ data: new Blob(['x']), headers: { 'content-disposition': 'attachment; filename="a.json"' } });
    await downloadFile('/notes/export?format=json', 'b.json');
    expect(clicked[0].download).toBe('a.json');
  });
});
