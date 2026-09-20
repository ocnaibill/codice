import { describe, it, expect } from 'vitest';
import { completionFor } from './progressRules';

describe('completionFor', () => {
  it('says finished only at the end, and otherwise says nothing (never "not finished")', () => {
    expect(completionFor(100)).toBe(true);
    expect(completionFor(95)).toBe(true);
    expect(completionFor(94.9)).toBeUndefined();
    expect(completionFor(10)).toBeUndefined(); // going back must not reopen it
    expect(completionFor(0)).toBeUndefined();
    expect(completionFor(undefined)).toBeUndefined();
    expect(completionFor(null)).toBeUndefined();
  });
});
