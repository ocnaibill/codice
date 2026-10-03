import { describe, it, expect, afterEach, vi } from 'vitest';
import { copyText } from './copyText';

afterEach(() => {
  vi.restoreAllMocks();
  delete document.execCommand;
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
});

describe('copyText', () => {
  it('puts the text on the clipboard and says it could', async () => {
    const writeText = vi.fn().mockResolvedValue();
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    expect(await copyText('um trecho')).toBe(true);
    expect(writeText).toHaveBeenCalledWith('um trecho');
  });

  it('copies the old way when the clipboard is not there (a page that is not secure), and cleans up after itself', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
    let copied = null;
    let selected = null;
    document.execCommand = vi.fn((command) => {
      const field = document.querySelector('textarea');
      copied = field?.value;
      selected = field ? field.value.slice(field.selectionStart, field.selectionEnd) : null; // what the copy takes is what is selected
      return command === 'copy';
    });
    expect(await copyText('outro trecho')).toBe(true);
    expect(copied).toBe('outro trecho');
    expect(selected).toBe('outro trecho');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('copies the old way when the clipboard refuses', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('NotAllowed')) } });
    document.execCommand = vi.fn(() => true);
    expect(await copyText('x')).toBe(true);
    expect(document.execCommand).toHaveBeenCalledWith('copy');
  });

  it('says it could not when nothing works', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('no')) } });
    document.execCommand = vi.fn(() => false);
    expect(await copyText('x')).toBe(false);
    document.execCommand = vi.fn(() => { throw new Error('gone'); });
    expect(await copyText('x')).toBe(false);
    expect(document.querySelector('textarea')).toBeNull();
  });
});
