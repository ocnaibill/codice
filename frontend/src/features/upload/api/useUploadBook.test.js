import { describe, it, expect } from 'vitest';
import { describeUploadError } from './useUploadBook';

const failure = (status, data) => ({ response: { status, data } });

describe('describeUploadError', () => {
  it('points to the existing record when the bytes are already stored', () => {
    expect(describeUploadError(failure(409, { error: 'duplicate', title: 'Duna.epub', work_id: 4, retired: false })))
      .toBe('Already in the library as "Duna.epub".');
    expect(describeUploadError(failure(409, { title: 'Duna.epub', retired: true })))
      .toContain('retired from the library');
  });

  it('relays what the server says about content, size and permission', () => {
    expect(describeUploadError(failure(415, 'the content is not a valid .epub file (wrong mimetype)\n')))
      .toBe('the content is not a valid .epub file (wrong mimetype)');
    expect(describeUploadError(failure(413, 'File is too large'))).toBe('The file is too large.');
    expect(describeUploadError(failure(403, 'Forbidden'))).toBe('Only administrators can add books.');
    expect(describeUploadError(failure(400, 'Unsupported file format\n'))).toBe('Unsupported file format');
  });

  it('distinguishes a timeout and an unreachable server from a rejection', () => {
    expect(describeUploadError({ code: 'ECONNABORTED' })).toBe('The upload timed out.');
    expect(describeUploadError({})).toBe('Could not reach the server.');
    expect(describeUploadError(failure(500, 'boom'))).toBe('Error uploading file.');
  });
});
