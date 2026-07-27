import { describe, it, expect, beforeEach } from 'vitest';
import { authenticatedUrl, wsUrl } from '../api.js';

// Mock localStorage
beforeEach(() => {
  const store = {};
  window.localStorage = {
    getItem: (key) => store[key] || null,
    setItem: (key, value) => { store[key] = value; },
    removeItem: (key) => { delete store[key]; },
    clear: () => { Object.keys(store).forEach(k => delete store[k]); },
  };
});

describe('authenticatedUrl', () => {
  it('returns path unchanged when no token', () => {
    expect(authenticatedUrl('/covers/test.jpg')).toBe('/covers/test.jpg');
  });

  it('appends token as query param', () => {
    localStorage.setItem('codice_token', 'my-jwt-token');
    const result = authenticatedUrl('/covers/test.jpg');
    expect(result).toContain('?token=');
    expect(result).toContain(encodeURIComponent('my-jwt-token'));
  });

  it('uses & separator when path already has query params', () => {
    localStorage.setItem('codice_token', 'token123');
    const result = authenticatedUrl('/path?existing=1');
    expect(result).toContain('&token=');
  });
});

describe('wsUrl', () => {
  it('uses ws:// and current host for http protocol', () => {
    localStorage.setItem('codice_token', 'mytoken');
    const result = wsUrl('/ws');
    // jsdom default: protocol=http:, host=localhost:3000
    expect(result).toMatch(/^ws:\/\/localhost:3000\/ws\?token=mytoken$/);
  });

  it('uses wss:// and current host for https protocol', () => {
    Object.defineProperty(window, 'location', {
      value: { protocol: 'https:', host: 'myserver.com' },
      writable: true,
      configurable: true,
    });
    localStorage.setItem('codice_token', 'tok');
    const result = wsUrl('/ws');
    expect(result).toBe('wss://myserver.com/ws?token=tok');
    // restore
    Object.defineProperty(window, 'location', {
      value: { protocol: 'http:', host: 'localhost:3000' },
      writable: true,
      configurable: true,
    });
  });
});
