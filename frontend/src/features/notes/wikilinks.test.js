import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { findLinks, linkIndex, linkify } from './wikilinks';

// The same cases the server's reader is tested with: a link must be shown the way it is kept.
const cases = JSON.parse(
  readFileSync(resolve(process.cwd(), '../backend/internal/graph/testdata/wikilinks.json'), 'utf8')
);

describe('findLinks reads the way the server does', () => {
  it('has the shared cases', () => {
    expect(cases.length).toBeGreaterThanOrEqual(30);
  });
  it.each(cases.map((c) => [c.name, c]))('%s', (_name, c) => {
    const got = findLinks(c.text);
    expect(got.map((l) => ({ name: l.name, label: l.label }))).toEqual(c.links);
    got.forEach((l) => {
      expect(c.text.slice(l.start, l.start + 2)).toBe('[[');
      expect(c.text.slice(l.end - 2, l.end)).toBe(']]');
    });
  });
});

describe('linkify', () => {
  it('turns each link into a link to its place, and touches nothing else', () => {
    const text = 'Antes [[A]] meio [[B|o b]] fim `[[C]]`';
    expect(linkify(text, findLinks(text))).toBe('Antes [A](wikilink:0) meio [o b](wikilink:1) fim `[[C]]`');
  });
  it('leaves a text with no link as it is', () => {
    expect(linkify('nada **aqui**', [])).toBe('nada **aqui**');
  });
  it('escapes what the shown text holds, so that it is not read as Markdown', () => {
    const text = '[[A|*x* _y_ `z` [w] <b>]]';
    // The bracket inside makes it no link at all; without it the rest is escaped.
    expect(findLinks(text)).toEqual([]);
    const t2 = '[[A|*x* _y_ `z` <b> & ~s~]]';
    expect(linkify(t2, findLinks(t2))).toBe('[\\*x\\* \\_y\\_ \\`z\\` \\<b\\> \\& \\~s\\~](wikilink:0)');
  });
  it('keeps the accents and the characters outside the first plane', () => {
    const text = '𝒜 [[Átomo]] 😀 [[B]]';
    expect(linkify(text, findLinks(text))).toBe('𝒜 [Átomo](wikilink:0) 😀 [B](wikilink:1)');
  });
});

describe('linkIndex', () => {
  it('says which link an address is', () => {
    expect(linkIndex('wikilink:3')).toBe(3);
    expect(linkIndex('https://exemplo.org')).toBe(-1);
    expect(linkIndex(undefined)).toBe(-1);
  });
});
