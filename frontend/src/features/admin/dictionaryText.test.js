import { describe, it, expect } from 'vitest';
import { coverageLabel, languageLabel, levelLabel, percent, progressLine, readyLine, sizeLine, sourceDay } from './dictionaryText';

describe('the languages and how well a package covers them', () => {
  it('names the languages of the library in Portuguese', () => {
    expect(['pt', 'en', 'es', 'fr', 'de', 'it', 'ja', 'zh'].map(languageLabel)).toEqual([
      'Português', 'Inglês', 'Espanhol', 'Francês', 'Alemão', 'Italiano', 'Japonês', 'Chinês',
    ]);
    expect(languageLabel('xx')).toBe('xx');
  });

  it('says what each level of coverage means for a person', () => {
    expect(levelLabel('complete')).toBe('completo');
    expect(levelLabel('partial')).toBe('parcial');
    expect(levelLabel('weak')).toBe('só tradução');
    expect(levelLabel('x')).toBe('x');
    expect(coverageLabel({ code: 'ja', level: 'weak' })).toBe('Japonês · só tradução');
  });
});

describe('percent', () => {
  it('is a whole number from 0 to 100', () => {
    expect(percent(0)).toBe(0);
    expect(percent(0.456)).toBe(46);
    expect(percent(0.004)).toBe(0);
    expect(percent(1)).toBe(100);
  });

  it('keeps what is not a fraction in range', () => {
    expect(percent(-1)).toBe(0);
    expect(percent(7)).toBe(100);
    expect(percent(undefined)).toBe(0);
    expect(percent(null)).toBe(0);
    expect(percent('x')).toBe(0);
    expect(percent(NaN)).toBe(0);
  });
});

describe('sizeLine', () => {
  it('says what is downloaded, and what it takes in the database when it is known', () => {
    expect(sizeLine({ downloadBytes: 37158613 })).toBe('Download de 35,4 MB');
    expect(sizeLine({ downloadBytes: 37158613, storageBytes: 326000000 })).toBe('Download de 35,4 MB · ocupa cerca de 310,9 MB no banco');
    expect(sizeLine({ downloadBytes: 733854991, storageBytes: 0 })).toBe('Download de 699,9 MB');
  });
});

describe('progressLine', () => {
  it('says the package is waiting for the worker before anything starts', () => {
    expect(progressLine({ stage: 'queued', progress: 0 })).toBe('Na fila, esperando o worker');
    expect(progressLine({ progress: 0 })).toBe('Na fila, esperando o worker');
  });

  it('says how much has been downloaded, of how much', () => {
    expect(progressLine({ stage: 'downloading', progress: 0.5, bytesDone: 18579306, bytesTotal: 37158613 })).toBe('Baixando: 50% (17,7 MB de 35,4 MB)');
  });

  it('says how much has been downloaded when the source did not say the size', () => {
    expect(progressLine({ stage: 'downloading', progress: 0, bytesDone: 1048576, bytesTotal: null })).toBe('Baixando: 1,0 MB');
  });

  it('says how many entries have been read, once there are some', () => {
    expect(progressLine({ stage: 'importing', progress: 0.5, entries: 118730 })).toBe('Importando: 50% (118.730 verbetes lidos)');
    expect(progressLine({ stage: 'importing', progress: 0, entries: 0 })).toBe('Importando: 0%');
  });

  it('says it is trying again, and why', () => {
    expect(progressLine({ stage: 'retrying', error: 'OSError: a conexão caiu' })).toBe('Tentando de novo: OSError: a conexão caiu');
    expect(progressLine({ stage: 'retrying' })).toBe('Tentando de novo');
  });
});

describe('readyLine', () => {
  it('says when it was installed, what it has and the date of the file', () => {
    const line = readyLine({ installedAt: '2026-10-03T10:00:00Z', entries: 456365, sourceDate: 'Mon, 28 Sep 2026 15:20:37 GMT' });
    expect(line).toMatch(/^Instalado em 03\/10\/2026/);
    expect(line).toContain('456.365 verbetes');
    expect(line).toContain('arquivo de 28/09/2026');
  });

  it('leaves out the date of the file when the source did not say it', () => {
    expect(readyLine({ installedAt: '2026-10-03T10:00:00Z', entries: 3, sourceDate: '' })).not.toContain('arquivo de');
    expect(readyLine({ installedAt: null, entries: 0 })).toContain('Instalado em —');
  });
});

describe('sourceDay', () => {
  it('reads the date of an HTTP header, and nothing else', () => {
    expect(sourceDay('Mon, 28 Sep 2026 15:20:37 GMT')).toBe('28/09/2026');
    expect(sourceDay('not a date')).toBe('');
    expect(sourceDay('')).toBe('');
    expect(sourceDay(undefined)).toBe('');
    expect(sourceDay(null)).toBe(''); // not the first day of 1970
  });
});

import { BIG_DOWNLOAD, isBig, matchesSearch, orderPackages } from './dictionaryText';

describe('the languages of the other editions', () => {
  it('names every language of the Wiktionaries the catalog has', () => {
    expect(['cs', 'nl', 'el', 'id', 'ko', 'ku', 'ms', 'pl', 'ru', 'th', 'tr', 'vi'].map(languageLabel)).toEqual([
      'Tcheco', 'Holandês', 'Grego', 'Indonésio', 'Coreano', 'Curdo', 'Malaio', 'Polonês', 'Russo', 'Tailandês', 'Turco', 'Vietnamita',
    ]);
  });
});

describe('matchesSearch', () => {
  const french = { name: 'Wikcionário em francês', languages: [{ code: 'fr', level: 'complete' }] };
  const pt = { name: 'Wikcionário em português', languages: [{ code: 'pt', level: 'complete' }, { code: 'ja', level: 'weak' }, { code: 'zh', level: 'weak' }] };

  it('finds a package by the name of the language, with no case and no accent', () => {
    expect(matchesSearch(french, 'francês')).toBe(true);
    expect(matchesSearch(french, 'FRANCES')).toBe(true);
    expect(matchesSearch(french, 'fran')).toBe(true);
    expect(matchesSearch(french, 'alemão')).toBe(false);
  });

  it('finds a package by a language it covers, and by its code', () => {
    expect(matchesSearch(pt, 'japonês')).toBe(true);
    expect(matchesSearch(pt, 'chines')).toBe(true);
    expect(matchesSearch(french, 'fr')).toBe(true);
    expect(matchesSearch(pt, 'zh')).toBe(true);
    expect(matchesSearch(french, 'zh')).toBe(false);
  });

  it('finds every package when nothing is searched for', () => {
    for (const q of ['', '   ', undefined, null]) expect(matchesSearch(french, q), String(q)).toBe(true);
  });

  it('copes with a package that says no languages', () => {
    expect(matchesSearch({ name: 'x' }, 'x')).toBe(true);
    expect(matchesSearch({ name: 'x' }, 'y')).toBe(false);
  });
});

describe('orderPackages', () => {
  it('puts what is installed, being installed or failed before what can be installed, and keeps the order inside each', () => {
    const list = [{ id: 'a', state: 'available' }, { id: 'b', state: 'ready' }, { id: 'c', state: 'available' }, { id: 'd', state: 'installing' }, { id: 'e', state: 'failed' }];
    expect(orderPackages(list).map((p) => p.id)).toEqual(['b', 'd', 'e', 'a', 'c']);
  });

  it('does not change the list it is given', () => {
    const list = [{ id: 'a', state: 'available' }, { id: 'b', state: 'ready' }];
    orderPackages(list);
    expect(list.map((p) => p.id)).toEqual(['a', 'b']);
  });
});

describe('isBig', () => {
  it('is a download of 200 MB or more', () => {
    expect(BIG_DOWNLOAD).toBe(209715200);
    expect(isBig({ downloadBytes: 209715200 })).toBe(true);
    expect(isBig({ downloadBytes: 209715199 })).toBe(false);
    expect(isBig({ downloadBytes: 733854991 })).toBe(true);
    expect(isBig({ downloadBytes: 37158613 })).toBe(false);
    expect(isBig({})).toBe(false);
  });
});
