import { api } from './api';

/**
 * Fetches a file the API produces on demand (an export), with the session token that cannot go in a link:
 * { blob, name }, to be shown first and saved afterwards, without asking for it twice.
 */
export async function fetchFile(url, fallbackName) {
  const response = await api.get(url, { responseType: 'blob', timeout: 60000 });
  const named = /filename="([^"]+)"/.exec(response.headers?.['content-disposition'] || '');
  return { blob: response.data, name: named ? named[1] : fallbackName };
}

/** Hands a file in memory to the browser as a saved file. */
export function saveBlob(blob, name) {
  const href = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = href;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(href), 1000);
}

/** Downloads a file the API produces on demand (an export) in one go. */
export async function downloadFile(url, fallbackName) {
  const { blob, name } = await fetchFile(url, fallbackName);
  saveBlob(blob, name);
}
