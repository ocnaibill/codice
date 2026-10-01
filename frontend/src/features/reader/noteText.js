export const KIND_LABEL = { note: 'Nota', highlight: 'Destaque', bookmark: 'Marcador' };

// What the server said, when it said something a person can act on ("a tag has at most 40
// characters"); a generic line otherwise.
export function reason(error, fallback) {
  const text = error?.response?.data;
  return typeof text === 'string' && text.trim() && error.response.status < 500 ? text.trim() : fallback;
}
