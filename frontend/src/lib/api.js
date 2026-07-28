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
  const baseUrl = import.meta.env.VITE_API_URL || '';
  
  // Se o path já tem http (ex: full URL) ignora o baseUrl, senão concatena
  const fullUrl = path.startsWith('http') ? path : `${baseUrl}${path}`;
  
  if (!token) return fullUrl;
  
  const separator = fullUrl.includes('?') ? '&' : '?';
  return `${fullUrl}${separator}token=${encodeURIComponent(token)}`;
}

/**
 * Returns a WebSocket URL for the given path, using the correct protocol and host
 */
export function wsUrl(path) {
  const baseUrl = import.meta.env.VITE_API_URL || '';
  const token = localStorage.getItem('codice_token');
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

  return `${wsBase}${path}?token=${encodeURIComponent(token)}`;
}