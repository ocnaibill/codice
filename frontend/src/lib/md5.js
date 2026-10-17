// MD5 of a text, as lower-case hex. The browser has no MD5 (its crypto does not offer one), and KOReader keeps the MD5 of the password as
// the key of the account (DEC-190), so the file of settings that is made in the browser needs it. Not for anything else: MD5 protects nothing.

const S = [7, 12, 17, 22, 5, 9, 14, 20, 4, 11, 16, 23, 6, 10, 15, 21];
const K = Array.from({ length: 64 }, (_, i) => Math.floor(Math.abs(Math.sin(i + 1)) * 2 ** 32) >>> 0);

const rotl = (x, n) => (x << n) | (x >>> (32 - n));

export function md5(text) {
  const bytes = new TextEncoder().encode(String(text));
  const length = bytes.length;
  const padded = new Uint8Array((((length + 8) >> 6) + 1) << 6);
  padded.set(bytes);
  padded[length] = 0x80;
  const view = new DataView(padded.buffer);
  view.setUint32(padded.length - 8, (length << 3) >>> 0, true);
  view.setUint32(padded.length - 4, Math.floor(length / 2 ** 29), true);

  let a0 = 0x67452301;
  let b0 = 0xefcdab89;
  let c0 = 0x98badcfe;
  let d0 = 0x10325476;
  for (let offset = 0; offset < padded.length; offset += 64) {
    const m = Array.from({ length: 16 }, (_, i) => view.getUint32(offset + i * 4, true));
    let a = a0;
    let b = b0;
    let c = c0;
    let d = d0;
    for (let i = 0; i < 64; i += 1) {
      let f;
      let g;
      if (i < 16) { f = (b & c) | (~b & d); g = i; }
      else if (i < 32) { f = (d & b) | (~d & c); g = (5 * i + 1) % 16; }
      else if (i < 48) { f = b ^ c ^ d; g = (3 * i + 5) % 16; }
      else { f = c ^ (b | ~d); g = (7 * i) % 16; }
      const next = d;
      d = c;
      c = b;
      b = (b + rotl((a + f + K[i] + m[g]) | 0, S[(i >> 4) * 4 + (i % 4)])) | 0;
      a = next;
    }
    a0 = (a0 + a) | 0;
    b0 = (b0 + b) | 0;
    c0 = (c0 + c) | 0;
    d0 = (d0 + d) | 0;
  }
  const out = new DataView(new ArrayBuffer(16));
  [a0, b0, c0, d0].forEach((v, i) => out.setUint32(i * 4, v >>> 0, true));
  return Array.from(new Uint8Array(out.buffer), (b) => b.toString(16).padStart(2, '0')).join('');
}
