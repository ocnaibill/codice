import axios from 'axios';

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '',
  timeout: 10000,
});

// Automatically attach JWT token to all outgoing HTTP requests
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('codice_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

/**
 * Returns a URL with JWT token as query param for assets that need auth
 * (e.g., <img src={authenticatedUrl('/covers/xxx.jpg')} />)
 */
export function authenticatedUrl(path) {
  const token = localStorage.getItem('codice_token');
  if (!token) return path;
  const separator = path.includes('?') ? '&' : '?';
  return `${path}${separator}token=${encodeURIComponent(token)}`;
}

/**
 * Returns a WebSocket URL for the given path, using the correct protocol and host
 */
export function wsUrl(path) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const token = localStorage.getItem('codice_token');
  return `${protocol}//${host}${path}?token=${encodeURIComponent(token)}`;
}