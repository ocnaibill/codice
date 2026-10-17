import { describe, it, expect } from 'vitest';
import { catalogAddress, isLocalAddress, koreaderOpdsFile, koreaderSyncFile, kosyncAddress, luaString } from './useAppTokens';
import { md5 } from '../../../lib/md5';

const at = (url) => new URL(url);

describe('catalogAddress', () => {
  it('is the catalog on the same origin the person already uses', () => {
    expect(catalogAddress(at('https://livros.example.com/?x=1'))).toBe('https://livros.example.com/opds/v1.2/catalog');
    expect(catalogAddress(at('http://192.168.1.20:8080/'))).toBe('http://192.168.1.20:8080/opds/v1.2/catalog');
  });
});

describe('isLocalAddress', () => {
  it('is true for the addresses only this computer can reach', () => {
    for (const url of ['http://localhost:8080/', 'http://127.0.0.1:8080/', 'http://127.1.2.3/', 'http://[::1]:8080/', 'http://codice.localhost/']) {
      expect(isLocalAddress(at(url)), url).toBe(true);
    }
  });

  it('is false for anything a phone on the network could reach', () => {
    for (const url of ['http://192.168.1.20:8080/', 'https://livros.example.com/', 'http://codice.lan/', 'http://localhost.example.com/', 'http://127.example.com/']) {
      expect(isLocalAddress(at(url)), url).toBe(false);
    }
  });
});

describe('the file of the catalog for KOReader', () => {
  it('writes a Lua string with what would break it escaped', () => {
    expect(luaString('Códice')).toBe('"Códice"');
    expect(luaString('a "b" \\ c')).toBe('"a \\"b\\" \\\\ c"');
    expect(luaString('uma\nlinha')).toBe('"uma\\nlinha"');
  });

  it('has the catalog, the user and the password without the spaces', () => {
    const text = koreaderOpdsFile({ address: 'https://livros.example.com/opds/v1.2/catalog', username: 'maria', token: '7k2m 9xqa r4bd h8tw' });
    expect(text).toContain('return {');
    expect(text).toContain('["servers"] = {');
    expect(text).toContain('["title"] = "Códice",');
    expect(text).toContain('["url"] = "https://livros.example.com/opds/v1.2/catalog",');
    expect(text).toContain('["username"] = "maria",');
    expect(text).toContain('["password"] = "7k2m9xqar4bdh8tw",');
  });

  it('does not let a name written with a quote end the string early', () => {
    const text = koreaderOpdsFile({ address: 'http://x/opds', username: 'a"b', token: 'abcd efgh jkmn pqrs' });
    expect(text).toContain('["username"] = "a\\"b",');
  });
});

describe('the file of the sync for KOReader', () => {
  const file = (token = '7k2m 9xqa r4bd h8tw') => koreaderSyncFile({ address: 'https://livros.example.com/kosync', username: 'maria', token });

  it('is under /kosync of the same site', () => {
    expect(kosyncAddress(at('https://livros.example.com/?x=1'))).toBe('https://livros.example.com/kosync');
  });

  it('has the server, the user, the MD5 of the password (as typed, without spaces) and every setting of the plugin', () => {
    const text = file();
    expect(text).toContain('["settings"] = {');
    expect(text).toContain('["custom_server"] = "https://livros.example.com/kosync",');
    expect(text).toContain('["username"] = "maria",');
    expect(text).toContain(`["userkey"] = "${md5('7k2m9xqar4bdh8tw')}",`);
    for (const line of ['["auto_sync"] = true,', '["sync_forward"] = 1,', '["sync_backward"] = 3,', '["checksum_method"] = 0,', '["send_metadata"] = false,']) {
      expect(text).toContain(line);
    }
    expect(text).not.toContain('7k2m9xqar4bdh8tw'); // the password is not in it, only what KOReader keeps of it
  });

  it('gives the same key however the password was shown: capitals, dashes, spaces', () => {
    const key = (t) => file(t).match(/\["userkey"\] = "([0-9a-f]{32})"/)[1];
    expect(key('7K2M-9XQA-R4BD-H8TW')).toBe(key('7k2m9xqar4bdh8tw'));
    expect(key('7k2m 9xqa r4bd h8tw')).toBe(key('7k2m9xqar4bdh8tw'));
  });
});
