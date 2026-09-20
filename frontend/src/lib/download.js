import { api } from './api';

/**
 * Downloads a file the API produces on demand (an export). The session token cannot go in a
 * link, so the file is fetched with it and handed to the browser as a saved file.
 */
export async function downloadFile(url, fallbackName) {
  const response = await api.get(url, { responseType: 'blob', timeout: 60000 });
  const named = /filename="([^"]+)"/.exec(response.headers?.['content-disposition'] || '');
  const href = URL.createObjectURL(response.data);
  const link = document.createElement('a');
  link.href = href;
  link.download = named ? named[1] : fallbackName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(href), 1000);
}
