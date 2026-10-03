/** Puts a text on the clipboard. Says whether it could: a browser may refuse (no permission, an insecure page). */
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // The old way, for a page that is not secure (a server on a local network, reached by address).
    const field = document.createElement('textarea');
    try {
      field.value = text;
      field.setAttribute('readonly', '');
      field.style.position = 'fixed';
      field.style.opacity = '0';
      document.body.appendChild(field);
      field.select();
      return !!document.execCommand('copy');
    } catch {
      return false;
    } finally {
      field.remove();
    }
  }
}
