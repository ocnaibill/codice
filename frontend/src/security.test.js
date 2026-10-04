import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, it, expect } from 'vitest';

// What is read from a file must not run as code on the origin of the app: a book that carries a script could read the token of the
// session (localStorage) or ask the API as the person. These are the rules of the source that keep it so.
const walk = (dir) => readdirSync(dir).flatMap((name) => {
  const path = join(dir, name);
  return statSync(path).isDirectory() ? walk(path) : [path];
});
const SOURCES = walk('src').filter((f) => /\.jsx?$/.test(f) && !/\.test\.jsx?$/.test(f));
const read = (file) => readFileSync(file, 'utf8');

describe('what a file can do on the origin of the app', () => {
  it('never allows scripts in the page of a book, nor in anything else that renders content of a file', () => {
    const found = SOURCES.flatMap((f) => [...read(f).matchAll(/allowScriptedContent\s*:\s*([^,}\s]+)/g)].filter((m) => m[1] !== 'false').map((m) => `${f}: ${m[0]}`));
    expect(found).toEqual([]);
  });

  it('never turns the text of a file into HTML the browser would run', () => {
    const found = SOURCES.filter((f) => /dangerouslySetInnerHTML|rehype-raw|rehypeRaw|allowDangerousHtml|\.innerHTML\s*=|srcdoc\s*=/.test(read(f)));
    expect(found).toEqual([]);
  });

  it('keeps the reader of EPUB with the scripts off, in the place where it opens the book', () => {
    const viewer = read('src/features/reader/components/viewers/EpubViewer.jsx');
    expect(viewer).toMatch(/book\.renderTo\([\s\S]*?allowScriptedContent:\s*false/);
  });
});

// The policy the browser enforces on the page is a file of the frontend image, so these are the rules of that file: the page
// may run only its own scripts, and every answer of the server carries the headers (a location that sets its own add_header
// would silently lose the ones of the server, so each one includes the snippet).
describe('the content security policy of the frontend server', () => {
  const csp = readFileSync('csp.conf', 'utf8');
  const policy = csp.split('\n').filter((l) => !l.trim().startsWith('#')).join(' ');
  const directive = (name) => (policy.match(new RegExp(`${name}\\s+([^;"]*)`)) ?? [])[1]?.trim();
  const nginx = readFileSync('nginx.conf', 'utf8');
  const headers = readFileSync('security-headers.conf', 'utf8');

  it('lets the page run only scripts of its own origin, never inline nor eval', () => {
    expect(directive('script-src')).toBe("'self'");
    expect(policy).not.toMatch(/unsafe-eval|unsafe-inline'[^;]*script|wasm-unsafe-eval/);
    expect(directive('default-src')).toBe("'self'");
  });

  it('allows no plugin, no other base, no form to another place, and no framing by another site', () => {
    expect(directive('object-src')).toBe("'none'");
    expect(directive('base-uri')).toBe("'self'");
    expect(directive('form-action')).toBe("'self'");
    expect(directive('frame-ancestors')).toBe("'self'");
  });

  it('keeps the requests of the page on its own origin, with the socket of live updates', () => {
    expect(directive('connect-src')).toBe("'self' ws://$http_host wss://$http_host");
    expect(directive('img-src')).not.toMatch(/https?:|\*/);
    expect(policy).not.toMatch(/https?:\/\/|\s\*[\s;"]/);
  });

  it('is sent on the page, and every location of the server includes the common headers', () => {
    const blocks = [...nginx.matchAll(/^\s*location\s[^{]*\{([\s\S]*?)^\s{4}\}/gm)].map((m) => m[1]);
    expect(blocks.length).toBeGreaterThanOrEqual(5);
    for (const block of blocks) expect(block).toContain('include /etc/nginx/snippets/security-headers.conf;');
    const page = blocks.find((b) => b.includes('try_files /index.html'));
    expect(page).toContain('include /etc/nginx/snippets/csp.conf;');
  });

  it('puts both snippets in the image where the server reads them', () => {
    const dockerfile = readFileSync('Dockerfile', 'utf8');
    expect(dockerfile).toMatch(/COPY\s+security-headers\.conf\s+csp\.conf\s+\/etc\/nginx\/snippets\//);
  });

  it('sets the headers that keep a file from being taken for something else or leaking the address', () => {
    expect(headers).toMatch(/X-Content-Type-Options "nosniff"/);
    expect(headers).toMatch(/Referrer-Policy "same-origin"/);
    expect(headers).toMatch(/X-Frame-Options "SAMEORIGIN"/);
    expect(headers).toMatch(/Permissions-Policy[^;]*camera=\(\)/);
  });
});
