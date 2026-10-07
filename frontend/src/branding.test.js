import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it, expect } from 'vitest';

// What the browser shows of the app outside the page: the icon of the tab and of the home screen of a phone, and the colour of
// the bar of the browser. (#179: the icon was the one of the tool that made the project, not the logo.)
const html = readFileSync('index.html', 'utf8');
const nginx = readFileSync('nginx.conf', 'utf8');
const css = readFileSync(join('src', 'index.css'), 'utf8');
const brand = css.match(/--color-brand:\s*(#[0-9a-fA-F]{6})/)[1];

describe('the icon and the colour of the app in the browser', () => {
  it('links an icon that is the book of the logo in the colour of the brand, not the one of the build tool', () => {
    const href = html.match(/<link rel="icon"[^>]*href="([^"]+)"/)[1];
    const svg = readFileSync(join('public', href.replace(/^\//, '')), 'utf8');
    expect(svg).toContain(brand);
    expect(svg).toContain('M12 5v15M12 5C8 2 3 3 2 4v15c4-2 7-1 10 1 3-2 6-3 10-1V4c-1-1-6-2-10 1Z'); // the path of the book of LibraryIcon
    expect(svg).not.toContain('#863bff'); // the purple of the template
  });

  it('has a PNG for the home screen of a phone, 180 by 180', () => {
    const href = html.match(/<link rel="apple-touch-icon"[^>]*href="([^"]+)"/)[1];
    const file = join('public', href.replace(/^\//, ''));
    expect(existsSync(file)).toBe(true);
    const png = readFileSync(file);
    expect(png.subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a');
    expect([png.readUInt32BE(16), png.readUInt32BE(20)]).toEqual([180, 180]);
  });

  it('colours the bar of the browser in the brand colour', () => {
    expect(html).toContain(`<meta name="theme-color" content="${brand}" />`);
  });

  it('keeps the book of the logo in the icon the same as the one of the library', () => {
    const icons = readFileSync(join('src', 'components', 'ui', 'LibraryIcon.jsx'), 'utf8');
    expect(icons).toContain('M12 5v15M12 5C8 2 3 3 2 4v15c4-2 7-1 10 1 3-2 6-3 10-1V4c-1-1-6-2-10 1Z');
  });

  it('lets the server of the container hand out the icons it links (its list of files is closed, and a file left out is a 404)', () => {
    const served = nginx.match(/location ~ \^\/\(([^)]+)\)\$ \{\s*try_files \$uri =404;/)[1].split('|').map((f) => f.replace(/\\/g, ''));
    for (const rel of [html.match(/<link rel="icon"[^>]*href="\/([^"]+)"/)[1], html.match(/<link rel="apple-touch-icon"[^>]*href="\/([^"]+)"/)[1]]) {
      expect(served, rel).toContain(rel);
    }
  });
});
