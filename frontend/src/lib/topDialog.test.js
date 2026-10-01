import { describe, it, expect, afterEach } from 'vitest';
import { isTopmostDialog } from './topDialog';

const made = [];
const dialog = () => {
  const el = document.createElement('div');
  el.setAttribute('role', 'dialog');
  document.body.appendChild(el);
  made.push(el);
  return el;
};
afterEach(() => made.splice(0).forEach((el) => el.remove()));

describe('isTopmostDialog', () => {
  it('is the last dialog in the document, and only that one', () => {
    const under = dialog();
    const over = dialog();
    expect(isTopmostDialog(over)).toBe(true);
    expect(isTopmostDialog(under)).toBe(false);
  });

  it('is not for something that is not there', () => {
    dialog();
    expect(isTopmostDialog(null)).toBe(false);
    expect(isTopmostDialog(document.createElement('div'))).toBe(false);
  });
});
