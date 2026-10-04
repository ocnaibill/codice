/**
 * What a browser says of itself, said the way a person names their devices: "Firefox em Windows". It is for
 * telling one's own devices apart in the list of sessions, not for knowing exactly what they are, so a text
 * it does not know is "Dispositivo desconhecido" and nothing is guessed. Order matters: most browsers
 * carry the name of another in their text (Edge and Opera say Chrome, Chrome says Safari).
 */
const BROWSERS = [
  ['Edge', /\bEdg(?:e|A|iOS)?\//],
  ['Opera', /\bOPR\/|\bOpera\b/],
  ['Samsung Internet', /\bSamsungBrowser\//],
  ['Firefox', /\bFirefox\/|\bFxiOS\//],
  ['Chrome', /\bChrome\/|\bCriOS\//],
  ['Safari', /\bSafari\//],
];

function systemOf(text) {
  if (/\biPhone\b/.test(text)) return 'iPhone';
  if (/\biPad\b/.test(text)) return 'iPad';
  if (/\bAndroid\b/.test(text)) return 'Android';
  if (/\bWindows\b/.test(text)) return 'Windows';
  if (/\bCrOS\b/.test(text)) return 'ChromeOS';
  if (/\bMacintosh\b|\bMac OS X\b/.test(text)) return 'macOS';
  if (/\bLinux\b|\bX11\b/.test(text)) return 'Linux';
  return null;
}

export function describeUserAgent(userAgent) {
  const text = typeof userAgent === 'string' ? userAgent : '';
  const browser = BROWSERS.find(([, pattern]) => pattern.test(text))?.[0] ?? null;
  const system = systemOf(text);
  let kind = 'computer';
  if (system === 'iPhone' || (system === 'Android' && /\bMobile\b/.test(text))) kind = 'phone';
  else if (system === 'iPad' || system === 'Android') kind = 'tablet';
  const label = browser && system ? `${browser} em ${system}` : browser || system || 'Dispositivo desconhecido';
  return { browser, system, kind, label };
}
