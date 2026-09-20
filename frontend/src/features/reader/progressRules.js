// When a file counts as finished (DEC-077): the last page, 95% of it, or the end of the audio.
// A viewer reports `true` when it gets there and says nothing otherwise: it never reports
// "not finished" just because the person went back, because finishing is a fact that only an
// explicit action undoes.
export const FINISHED_AT_PERCENT = 95;

export function completionFor(percent) {
  return percent != null && percent >= FINISHED_AT_PERCENT ? true : undefined;
}
