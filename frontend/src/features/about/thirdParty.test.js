import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { THIRD_PARTY } from './thirdParty';

const pkg = JSON.parse(readFileSync(join(process.cwd(), 'package.json'), 'utf8'));
const dependencies = Object.keys({ ...pkg.dependencies, ...pkg.devDependencies });

describe('third-party list', () => {
  it('names, says the license and links every entry over https', () => {
    expect(THIRD_PARTY.length).toBeGreaterThan(5);
    for (const entry of THIRD_PARTY) {
      expect(entry.name.trim()).not.toBe('');
      expect(entry.what.trim()).not.toBe('');
      expect(entry.license.trim()).not.toBe('');
      expect(entry.url).toMatch(/^https:\/\/[^/\s]+/);
    }
    expect(new Set(THIRD_PARTY.map((entry) => entry.name)).size).toBe(THIRD_PARTY.length);
  });

  it('only claims npm packages that the app really depends on', () => {
    for (const entry of THIRD_PARTY.filter((e) => e.package)) {
      expect(dependencies, entry.name).toContain(entry.package);
    }
  });

  it('names every font the app ships (the OFL asks for it)', () => {
    const fonts = dependencies.filter((name) => name.startsWith('@fontsource'));
    expect(fonts.length).toBeGreaterThan(0);
    const named = THIRD_PARTY.map((entry) => entry.package);
    for (const font of fonts) expect(named, font).toContain(font);
  });

  it('keeps the license of the Wiktionary data, which asks for attribution', () => {
    const wiktionary = THIRD_PARTY.find((entry) => entry.name === 'Wikcionário');
    expect(wiktionary.license).toContain('CC BY-SA');
  });
});
