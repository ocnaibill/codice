// What each admin screen says to someone who may look and not change (#77, "permissão insuficiente"): where the owner has
// controls, an admin has a note that says who has them, in the same words everywhere.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn(), patch: vi.fn() },
}));

import { api } from '../../lib/api';
import { mount } from './testUtils';
import { ProvidersTab } from './components/ProvidersTab';
import { DictionariesTab } from './components/DictionariesTab';
import { OcrTab } from './components/OcrTab';
import { StorageTab } from './components/StorageTab';
import { TrashTab } from './components/TrashTab';
import { SuggestionsTab } from './components/SuggestionsTab';

let view;
beforeEach(() => vi.clearAllMocks());
afterEach(() => view?.unmount());

const answers = {
  '/admin/metadata-providers': { data: [] },
  '/admin/dictionaries': { packages: [] },
  '/admin/ocr/settings': { enabled: true, available: true, language: 'por', languages: ['por'], engineLanguages: ['por'] },
  '/admin/ocr': { data: [] },
  '/admin/roots': { roots: [], managed: '/dados' },
  '/admin/trash': { data: [], policy: { enabled: true, days: 30 } },
  '/admin/suggestions': { data: [] },
};
const serve = () => api.get.mockImplementation(async (url) => ({ data: answers[url.split('?')[0]] ?? { data: [] } }));
const notes = () => [...document.body.querySelectorAll('[role="note"]')].map((n) => n.textContent);

const CASES = [
  ['os provedores', (isOwner) => <ProvidersTab isOwner={isOwner} />, 'Só o dono do acervo liga ou desliga os provedores.'],
  ['os dicionários', (isOwner) => <DictionariesTab isOwner={isOwner} />, 'Só o dono do acervo instala, atualiza e remove dicionários.'],
  ['o OCR', (isOwner) => <OcrTab isOwner={isOwner} />, 'Só o dono do acervo liga ou desliga o OCR e escolhe os idiomas.'],
  ['as pastas autorizadas', (isOwner) => <StorageTab isOwner={isOwner} />, 'Só o dono do acervo autoriza e remove pastas.'],
  ['a lixeira', (isOwner) => <TrashTab isOwner={isOwner} />, 'Só o dono do acervo muda isso e apaga de vez.'],
];

describe('the admin screens for an admin who is not the owner', () => {
  it.each(CASES)('say who can change %s, in a note', async (_name, element, text) => {
    serve();
    view = await mount(element(false));
    expect(notes()).toContain(text);
  });

  it.each(CASES)('say nothing of it to the owner, who can: %s', async (_name, element, text) => {
    serve();
    view = await mount(element(true));
    expect(notes()).not.toContain(text);
  });

  it('never say "owner" to a person: it is "o dono do acervo"', async () => {
    serve();
    for (const [, element] of CASES) {
      view = await mount(element(false));
      expect(document.body.textContent).not.toMatch(/\bowner\b/i);
      view.unmount();
    }
    view = await mount(<div />);
  });

  it('tell an admin of the queue that only the owner turns the providers on, in the same words', async () => {
    serve();
    api.get.mockImplementation(async (url) => ({ data: url.includes('metadata-providers') ? { data: [{ id: 'openlibrary', enabled: false }] } : { data: [] } }));
    view = await mount(<SuggestionsTab isOwner={false} onOpenProviders={() => {}} />);
    expect(document.body.textContent).toContain('Só o dono do acervo liga os provedores.');
    expect(document.body.textContent).not.toMatch(/\bowner\b/i);
  });
});
