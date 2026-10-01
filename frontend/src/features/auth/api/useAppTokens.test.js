import { describe, it, expect } from 'vitest';
import { catalogAddress, isLocalAddress } from './useAppTokens';

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
