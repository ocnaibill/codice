/**
 * The versions of a work, other than the one just finished, that the person has begun and not
 * finished (from the detail of the work). They are what "mark the whole work as finished" would
 * take out of Continue Reading, so they are what the question has to name.
 */
export function otherVersionsInProgress(work, finishedFileId) {
  const found = [];
  for (const edition of work?.editions ?? []) {
    for (const file of edition.files ?? []) {
      if (file.id === finishedFileId) continue;
      if (file.availability !== 'available' || !file.started || file.completed) continue;
      found.push({ file, edition });
    }
  }
  return found;
}
