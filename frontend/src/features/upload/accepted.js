// What the upload accepts (the server checks the content again): the extensions the file picker offers and that a file
// dropped on the window is held to.
export const ACCEPTED_EXTENSIONS = ['pdf', 'epub', 'cbz', 'cbr', 'txt', 'md', 'mobi', 'azw', 'azw3', 'mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav'];

export const ACCEPT_ATTRIBUTE = ACCEPTED_EXTENSIONS.map((ext) => `.${ext}`).join(',');

/** "Duna.EPUB" -> "epub"; "" when there is none. */
export function extensionOf(name) {
  const dot = String(name ?? '').lastIndexOf('.');
  return dot > 0 && dot < name.length - 1 ? name.slice(dot + 1).toLowerCase() : '';
}

export const isAccepted = (name) => ACCEPTED_EXTENSIONS.includes(extensionOf(name));
