import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it, expect } from 'vitest';

// The rules of the system of design (#77) that are checked by reading the source: they are what a person forgets, and what a
// review does not see.
const SRC = 'src';
const sources = (dir = SRC) =>
  readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.jsx?$/.test(name) && !/\.test\.jsx?$/.test(name) ? [path] : [];
  });

const FILES = sources();

describe('the colors of the app', () => {
  it('are the roles of the theme (brand, ink, danger, warning, success...) and never a color of the palette of the framework', () => {
    const loose = /\b(?:bg|text|border|ring|divide|from|via|to|placeholder|fill|stroke|outline|decoration|accent|caret)-(?:zinc|slate|gray|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b/g;
    const found = FILES.flatMap((file) => (readFileSync(file, 'utf8').match(loose) ?? []).map((m) => `${file}: ${m}`));
    expect(found).toEqual([]);
  });

  it('has the roles of color that a notice takes, in the theme', () => {
    const css = readFileSync(join(SRC, 'index.css'), 'utf8');
    for (const role of ['danger', 'danger-soft', 'warning', 'warning-soft', 'success', 'success-soft', 'info-soft']) {
      expect(css, role).toContain(`--color-${role}:`);
    }
  });
});

describe('a screen that cannot load what it shows', () => {
  it('offers to try again: every LoadError is given what to do', () => {
    const without = FILES.flatMap((file) => {
      const text = readFileSync(file, 'utf8');
      return [...text.matchAll(/<LoadError\b([^>]*)>/g)].filter((m) => !/\bonRetry=/.test(m[1])).map(() => file);
    });
    expect(without).toEqual([]);
  });

  it('is told what failed, so that a "no" is not offered a second try: every LoadError is given its error', () => {
    const without = FILES.flatMap((file) => {
      const text = readFileSync(file, 'utf8');
      return [...text.matchAll(/<LoadError\b([^>]*)>/g)].filter((m) => !/\berror=/.test(m[1])).map(() => file);
    });
    expect(without).toEqual([]);
  });

  it('is used by the screens, and not left only in the components that it was made in', () => {
    const users = FILES.filter((file) => /<LoadError\b/.test(readFileSync(file, 'utf8')) && !file.endsWith('LoadError.jsx'));
    expect(users.length).toBeGreaterThanOrEqual(15);
  });
});

describe('the text of the interface', () => {
  it('is in Portuguese where a person reads it: nothing of what the app says is left in English', () => {
    const english = /(?:>|["'`])(?:Something went wrong|An unexpected error|Loading |Failed to |Reload|Try again)[^<"'`]*/;
    const found = FILES.filter((file) => !file.includes('/lib/') && english.test(readFileSync(file, 'utf8').split('\n').filter((l) => !/console\.|^\s*(\/\/|\*)/.test(l)).join('\n')));
    expect(found).toEqual([]);
  });
});
