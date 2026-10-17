import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { md5 } from '../../../lib/md5';

const KEY = ['app-tokens'];

/** The accesses this account gave to reading apps (DEC-071): [{ id, name, scope, createdAt, lastUsedAt? }], newest first. */
export function useAppTokens() {
  return useQuery({
    queryKey: KEY,
    queryFn: async () => (await api.get('/auth/app-tokens')).data,
    // Belongs to one account: it is dropped as soon as the screen closes, so it can never be shown to the next
    // person who signs in on this browser.
    gcTime: 0,
  });
}

/**
 * Creates one; the answer carries the token, the only time it is ever shown. `scope` is "app" (the long token every reading app takes) or
 * "koreader" (sixteen symbols, made to be typed on an e-reader, DEC-190).
 */
export function useCreateAppToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ name, scope = 'app' }) => (await api.post('/auth/app-tokens', { name, scope })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

export function useRevokeAppToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id) => api.delete(`/auth/app-tokens/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KEY }),
  });
}

/** What an OPDS app is told to connect to: this same address, so it is the one the person already uses. */
export function catalogAddress(location = window.location) {
  return `${location.origin}/opds/v1.2/catalog`;
}

/** An address that only works on the machine that typed it: a phone or an e-reader cannot reach it. */
export function isLocalAddress(location = window.location) {
  const host = location.hostname.replace(/^\[|\]$/g, '');
  return host === 'localhost' || host.endsWith('.localhost') || host === '::1' || /^127(\.\d{1,3}){3}$/.test(host);
}

/** A text as a Lua string between double quotes. */
export function luaString(text) {
  const escapes = { '\\': '\\\\', '"': '\\"', '\n': '\\n', '\r': '\\r' };
  return `"${String(text).replace(/[\\"\n\r]/g, (c) => escapes[c])}"`;
}

/**
 * The file KOReader keeps its OPDS catalogs in (`settings/opds.lua`), with the Códice already in it: copied to the device, the person
 * types nothing on its keyboard. The password is the token as the person would type it, without the spaces.
 */
export function koreaderOpdsFile({ address, username, token, title = 'Códice' }) {
  const password = String(token).replace(/\s+/g, '');
  return [
    '-- Catálogo do Códice para o KOReader. Copie este arquivo para a pasta koreader/settings do aparelho.',
    'return {',
    '    ["servers"] = {',
    '        {',
    `            ["title"] = ${luaString(title)},`,
    `            ["url"] = ${luaString(address)},`,
    `            ["username"] = ${luaString(username)},`,
    `            ["password"] = ${luaString(password)},`,
    '        },',
    '    },',
    '}',
    '',
  ].join('\n');
}

/** The address KOReader is told as its server of sync (KOSync): the same site, under /kosync. */
export function kosyncAddress(location = window.location) {
  return `${location.origin}/kosync`;
}

/**
 * The file KOReader keeps its sync of progress in (`settings/kosync.lua`), with the Códice already in it: copied to the device, nothing is typed on
 * its keyboard. It carries all the settings of the plugin (a settings table that lacks one leaves it unset: the plugin reads the table whole),
 * with the sync automatic and a question when the Códice has a place that is more recent (what the plugin itself starts with). The key is the
 * MD5 of the password, which is what KOReader keeps and sends.
 */
export function koreaderSyncFile({ address, username, token }) {
  const password = String(token).replace(/[\s-]+/g, '').toLowerCase();
  return [
    '-- Sincronização de progresso do KOReader com o Códice. Copie este arquivo para a pasta koreader/settings do aparelho.',
    'return {',
    '    ["settings"] = {',
    `        ["custom_server"] = ${luaString(address)},`,
    `        ["username"] = ${luaString(username)},`,
    `        ["userkey"] = ${luaString(md5(password))},`,
    '        ["auto_sync"] = true,',
    '        ["sync_forward"] = 1,',
    '        ["sync_backward"] = 3,',
    '        ["checksum_method"] = 0,',
    '        ["send_metadata"] = false,',
    '    },',
    '}',
    '',
  ].join('\n');
}
