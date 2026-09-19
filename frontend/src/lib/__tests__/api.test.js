import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  api,
  authenticatedUrl,
  wsUrl,
  refreshAssetToken,
  clearAssetToken,
  UNAUTHORIZED_EVENT,
} from '../api.js';

beforeEach(() => {
  const store = {};
  window.localStorage = {
    getItem: (key) => store[key] || null,
    setItem: (key, value) => { store[key] = value; },
    removeItem: (key) => { delete store[key]; },
    clear: () => { Object.keys(store).forEach(k => delete store[k]); },
  };
  clearAssetToken();
});

afterEach(() => {
  vi.restoreAllMocks();
  clearAssetToken();
});

const inAnHour = () => new Date(Date.now() + 15 * 60 * 1000).toISOString();

describe('authenticatedUrl', () => {
  it('returns path unchanged when there is no asset token', () => {
    expect(authenticatedUrl('/covers/test.jpg')).toBe('/covers/test.jpg');
  });

  it('appends the asset token as rt, never the session token', async () => {
    localStorage.setItem('codice_token', 'session-jwt');
    vi.spyOn(api, 'post').mockResolvedValue({ data: { token: 'asset-tok', expiresAt: inAnHour() } });
    await refreshAssetToken();

    const result = authenticatedUrl('/covers/test.jpg');
    expect(result).toBe('/covers/test.jpg?rt=asset-tok');
    expect(result).not.toContain('token=');
    expect(result).not.toContain('session-jwt');
  });

  it('uses & separator when path already has query params', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { token: 'asset-tok', expiresAt: inAnHour() } });
    await refreshAssetToken();
    expect(authenticatedUrl('/path?existing=1')).toBe('/path?existing=1&rt=asset-tok');
  });

  it('asks the server for the assets scope', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { token: 't', expiresAt: inAnHour() } });
    await refreshAssetToken();
    expect(post).toHaveBeenCalledWith('/auth/resource-token', { scope: 'assets' });
  });

  it('stops attaching the token after clearAssetToken (logout)', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { token: 'asset-tok', expiresAt: inAnHour() } });
    await refreshAssetToken();
    clearAssetToken();
    expect(authenticatedUrl('/covers/test.jpg')).toBe('/covers/test.jpg');
  });
});

describe('wsUrl', () => {
  it('builds a ws:// URL with a short-lived ticket, not the session token', async () => {
    localStorage.setItem('codice_token', 'session-jwt');
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { token: 'ticket-1' } });

    const result = await wsUrl('/ws');

    expect(post).toHaveBeenCalledWith('/auth/resource-token', { scope: 'ws' });
    expect(result).toBe('ws://localhost:3000/ws?ticket=ticket-1');
    expect(result).not.toContain('session-jwt');
    expect(result).not.toContain('token=');
  });

  it('rejects when the server refuses the ticket', async () => {
    vi.spyOn(api, 'post').mockRejectedValue(new Error('401'));
    await expect(wsUrl('/ws')).rejects.toThrow();
  });
});

describe('session loss', () => {
  const failWith = (status) => {
    api.defaults.adapter = (config) =>
      Promise.reject({ config, response: { status, data: {} }, isAxiosError: true });
  };

  afterEach(() => {
    delete api.defaults.adapter;
  });

  it('clears the token and announces it on a 401 from a normal call', async () => {
    localStorage.setItem('codice_token', 'stale');
    const onLost = vi.fn();
    window.addEventListener(UNAUTHORIZED_EVENT, onLost);
    failWith(401);

    await expect(api.get('/works')).rejects.toBeTruthy();

    expect(localStorage.getItem('codice_token')).toBeNull();
    expect(onLost).toHaveBeenCalledTimes(1);
    window.removeEventListener(UNAUTHORIZED_EVENT, onLost);
  });

  it('does not treat a failed login as a lost session', async () => {
    const onLost = vi.fn();
    window.addEventListener(UNAUTHORIZED_EVENT, onLost);
    failWith(401);

    await expect(api.post('/auth/login', {})).rejects.toBeTruthy();

    expect(onLost).not.toHaveBeenCalled();
    window.removeEventListener(UNAUTHORIZED_EVENT, onLost);
  });

  it('keeps the session on other errors', async () => {
    localStorage.setItem('codice_token', 'still-good');
    failWith(403);

    await expect(api.put('/works/1', {})).rejects.toBeTruthy();

    expect(localStorage.getItem('codice_token')).toBe('still-good');
  });
});
