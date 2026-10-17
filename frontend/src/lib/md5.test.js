import { describe, it, expect } from 'vitest';
import { md5 } from './md5';

describe('md5', () => {
  it('is the MD5 of the test suite of RFC 1321', () => {
    expect(md5('')).toBe('d41d8cd98f00b204e9800998ecf8427e');
    expect(md5('a')).toBe('0cc175b9c0f1b6a831c399e269772661');
    expect(md5('abc')).toBe('900150983cd24fb0d6963f7d28e17f72');
    expect(md5('message digest')).toBe('f96b697d7cb7938d525a2f31aaf161d0');
    expect(md5('abcdefghijklmnopqrstuvwxyz')).toBe('c3fcd3d76192e4007dfb496cca67e13b');
    expect(md5('ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789')).toBe('d174ab98d277d9f5a5611c2c9f419d9f');
    expect(md5('12345678901234567890123456789012345678901234567890123456789012345678901234567890')).toBe('57edf4a22be3c955ac49da2e2107b67a');
  });

  it('is what a KOReader sent for a password typed on a Kindle (measured on 16/10/2026)', () => {
    expect(md5('teste123')).toBe('aa1bf4646de67fd9086cf6c79007026c');
  });

  it('reads a text as UTF-8, and gets the lengths around a block right', () => {
    expect(md5('Códice')).toBe('8945f2b4bd86f39839ae1e40ab72a744');
    for (const n of [55, 56, 57, 63, 64, 65, 119, 120, 121]) expect(md5('x'.repeat(n))).toMatch(/^[0-9a-f]{32}$/);
    expect(md5('a'.repeat(1000))).toBe('cabe45dcc9ae5b66ba86600cca6b8ba8');
  });
});
