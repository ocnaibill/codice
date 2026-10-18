/**
 * The files of what was dropped, folders included: a folder is walked, and each file keeps the path it had inside it
 * (`webkitRelativePath`, as the folder picker does), so that the name of the folder can name the series. Where the browser cannot walk a
 * folder, the files it gives are the answer.
 */
export async function filesFromDrop(dataTransfer) {
  // Both must be read before the first wait: the browser empties them when the event ends.
  const plain = [...(dataTransfer?.files ?? [])];
  const entries = [...(dataTransfer?.items ?? [])].map((item) => item.webkitGetAsEntry?.()).filter(Boolean);
  if (!entries.some((entry) => entry.isDirectory)) return plain;

  const out = [];
  const walk = async (entry, path) => {
    if (entry.isFile) {
      const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
      try {
        Object.defineProperty(file, 'webkitRelativePath', { value: path + file.name });
      } catch {
        // a browser that does not allow it: the file is still taken, without its folder
      }
      out.push(file);
    } else if (entry.isDirectory) {
      const reader = entry.createReader();
      for (;;) {
        const batch = await new Promise((resolve, reject) => reader.readEntries(resolve, reject));
        if (batch.length === 0) break;
        for (const child of batch) await walk(child, `${path}${entry.name}/`);
      }
    }
  };
  for (const entry of entries) await walk(entry, '');
  return out;
}
