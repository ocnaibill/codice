import { serverMessage } from '../../lib/serverMessage';

export const KIND_LABEL = { note: 'Nota', highlight: 'Destaque', bookmark: 'Marcador' };

// What the server said, when it said something a person can act on ("a tag has at most 40
// characters"); a generic line otherwise.
export function reason(error, fallback) {
  return serverMessage(error, fallback);
}
