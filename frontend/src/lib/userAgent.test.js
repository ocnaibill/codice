import { describe, it, expect } from 'vitest';
import { describeUserAgent } from './userAgent';

const UA = {
  firefoxWindows: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:130.0) Gecko/20100101 Firefox/130.0',
  chromeWindows: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36',
  edgeWindows: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0',
  operaWindows: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 OPR/114.0.0.0',
  safariMac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Safari/605.1.15',
  chromeMac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36',
  safariIphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Mobile/15E148 Safari/604.1',
  chromeIphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/129.0.6668.69 Mobile/15E148 Safari/604.1',
  firefoxIphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/130.0 Mobile/15E148 Safari/605.1.15',
  edgeIphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) EdgiOS/129.0 Mobile/15E148 Safari/605.1.15',
  safariIpad: 'Mozilla/5.0 (iPad; CPU OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Mobile/15E148 Safari/604.1',
  chromeAndroid: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36',
  chromeAndroidTablet: 'Mozilla/5.0 (Linux; Android 13; SM-X700) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36',
  samsung: 'Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/25.0 Chrome/121.0.0.0 Mobile Safari/537.36',
  firefoxAndroid: 'Mozilla/5.0 (Android 14; Mobile; rv:130.0) Gecko/130.0 Firefox/130.0',
  firefoxLinux: 'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0',
  chromeos: 'Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36',
};

describe('describeUserAgent', () => {
  it.each([
    ['firefoxWindows', 'Firefox em Windows'],
    ['chromeWindows', 'Chrome em Windows'],
    ['edgeWindows', 'Edge em Windows'],
    ['operaWindows', 'Opera em Windows'],
    ['safariMac', 'Safari em macOS'],
    ['chromeMac', 'Chrome em macOS'],
    ['safariIphone', 'Safari em iPhone'],
    ['chromeIphone', 'Chrome em iPhone'],
    ['firefoxIphone', 'Firefox em iPhone'],
    ['edgeIphone', 'Edge em iPhone'],
    ['safariIpad', 'Safari em iPad'],
    ['chromeAndroid', 'Chrome em Android'],
    ['samsung', 'Samsung Internet em Android'],
    ['firefoxAndroid', 'Firefox em Android'],
    ['firefoxLinux', 'Firefox em Linux'],
    ['chromeos', 'Chrome em ChromeOS'],
  ])('names %s as "%s"', (key, label) => {
    expect(describeUserAgent(UA[key]).label).toBe(label);
  });

  it('tells a phone, a tablet and a computer', () => {
    expect(describeUserAgent(UA.safariIphone).kind).toBe('phone');
    expect(describeUserAgent(UA.chromeAndroid).kind).toBe('phone');
    expect(describeUserAgent(UA.firefoxAndroid).kind).toBe('phone');
    expect(describeUserAgent(UA.safariIpad).kind).toBe('tablet');
    expect(describeUserAgent(UA.chromeAndroidTablet).kind).toBe('tablet');
    expect(describeUserAgent(UA.firefoxWindows).kind).toBe('computer');
    expect(describeUserAgent(UA.safariMac).kind).toBe('computer');
  });

  it('says what it knows of a text it half knows, and never guesses', () => {
    expect(describeUserAgent('Mozilla/5.0 (Windows NT 10.0)').label).toBe('Windows');
    expect(describeUserAgent('Firefox/130.0').label).toBe('Firefox');
    expect(describeUserAgent('curl/8.4.0').label).toBe('Dispositivo desconhecido');
  });

  it('is calm about nothing at all', () => {
    for (const nothing of [undefined, null, '', 42, {}]) {
      expect(describeUserAgent(nothing)).toEqual({ browser: null, system: null, kind: 'computer', label: 'Dispositivo desconhecido' });
    }
  });
});
