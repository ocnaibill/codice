import { describe, expect, it } from 'vitest';
import { ACCEPT_ATTRIBUTE, ACCEPTED_EXTENSIONS, extensionOf, isAccepted } from './accepted';

describe('what an upload accepts', () => {
  it('reads the extension of a name, whatever its case, and nothing from a name without one', () => {
    expect(extensionOf('Duna.EPUB')).toBe('epub');
    expect(extensionOf('a.b.c.cbz')).toBe('cbz');
    expect(extensionOf('semextensao')).toBe('');
    expect(extensionOf('.epub')).toBe('');
    expect(extensionOf('termina.')).toBe('');
    expect(extensionOf('')).toBe('');
    expect(extensionOf(undefined)).toBe('');
  });
  it('accepts books, comics, documents and audio, and refuses the rest', () => {
    for (const name of ['a.pdf', 'a.EPUB', 'a.cbz', 'a.cbr', 'a.txt', 'a.md', 'a.mobi', 'a.azw3', 'a.mp3', 'a.m4b', 'a.flac']) expect(isAccepted(name)).toBe(true);
    for (const name of ['a.exe', 'a.zip', 'a.docx', 'a', 'a.epub.exe']) expect(isAccepted(name)).toBe(false);
  });
  it('offers the same list to the file picker', () => {
    expect(ACCEPT_ATTRIBUTE.split(',')).toEqual(ACCEPTED_EXTENSIONS.map((e) => `.${e}`));
  });
});
