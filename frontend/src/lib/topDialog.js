// With dialogs open over one another (a confirmation over the metadata over the sheet of a work), Escape closes
// only the one on top: the last in the document.
export function isTopmostDialog(element) {
  const all = document.querySelectorAll('[role="dialog"]');
  return !!element && all[all.length - 1] === element;
}
