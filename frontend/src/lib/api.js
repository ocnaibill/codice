import axios from 'axios';

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '',
  timeout: 10000,
});

const TOKEN_KEY = 'codice_token';

/** Fired when the server no longer accepts our session (expired, revoked, blocked). */
export const UNAUTHORIZED_EVENT = 'codice:unauthorized';

// Automatically attach the session token to all outgoing HTTP requests
api.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY);
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// A 401 on anything but the login/setup calls means the session is gone.
api.interceptors.response.use(
  (response) => response,
  (error) => {
    const url = error?.config?.url || '';
    const isCredentialCall = url.startsWith('/auth/login') || url.startsWith('/auth/setup') || url.startsWith('/auth/link');
    if (error?.response?.status === 401 && !isCredentialCall) {
      localStorage.removeItem(TOKEN_KEY);
      clearAssetToken();
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
    }
    return Promise.reject(error);
  }
);

// Short-lived token for URLs the browser loads without an Authorization header
// (<img>, <audio>, download links). It is limited to reading assets and is
// renewed before it expires; the session token itself never goes in a URL.
let assetToken = null;
let assetRefreshTimer = null;

export function clearAssetToken() {
  assetToken = null;
  if (assetRefreshTimer) {
    clearTimeout(assetRefreshTimer);
    assetRefreshTimer = null;
  }
}

/** Fetches (and schedules the renewal of) the asset token. */
export async function refreshAssetToken() {
  const res = await api.post('/auth/resource-token', { scope: 'assets' });
  assetToken = res.data.token;

  const lifetime = new Date(res.data.expiresAt).getTime() - Date.now();
  const delay = Math.max(30_000, lifetime * 0.6);
  if (assetRefreshTimer) clearTimeout(assetRefreshTimer);
  assetRefreshTimer = setTimeout(() => {
    refreshAssetToken().catch(() => {});
  }, delay);
}

/**
 * Returns a URL that carries the asset token, for assets that need auth
 * (e.g., <img src={authenticatedUrl('/covers/xxx.jpg')} />)
 */
export function authenticatedUrl(path) {
  const baseUrl = import.meta.env.VITE_API_URL || '';

  // Se o path já tem http (ex: full URL) ignora o baseUrl, senão concatena
  const fullUrl = path.startsWith('http') ? path : `${baseUrl}${path}`;

  if (!assetToken) return fullUrl;

  const separator = fullUrl.includes('?') ? '&' : '?';
  return `${fullUrl}${separator}rt=${encodeURIComponent(assetToken)}`;
}

/**
 * Returns a WebSocket URL for the given path, using the correct protocol and host.
 * Browsers cannot set headers on a WebSocket, so this asks the server for a
 * 60-second single-purpose ticket instead of putting the session token in the URL.
 */
export async function wsUrl(path) {
  const baseUrl = import.meta.env.VITE_API_URL || '';
  let wsBase = '';

  if (baseUrl) {
    // Converte http:// para ws:// e https:// para wss:// baseando-se no VITE_API_URL
    wsBase = baseUrl.replace(/^http/, 'ws');
  } else {
    // Fallback pra mesma origem do frontend se não tiver API_URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    wsBase = `${protocol}//${host}`;
  }

  const res = await api.post('/auth/resource-token', { scope: 'ws' });
  const separator = path.includes('?') ? '&' : '?';
  return `${wsBase}${path}${separator}ticket=${encodeURIComponent(res.data.token)}`;
}
