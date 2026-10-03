// What every admin tab does when what it was loading does not come: it says so, offers to try again, and trying again asks
// the server again and shows what came (#77). The tabs say it in their own words and ask for their own things, so they are
// tried here together.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../../../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn(), patch: vi.fn() },
}));

import { api } from '../../../lib/api';
import { mount } from '../testUtils';
import { JobsTab } from './JobsTab';
import { AccountsTab } from './AccountsTab';
import { TrashTab } from './TrashTab';
import { DuplicatesTab } from './DuplicatesTab';
import { SuggestionsTab } from './SuggestionsTab';
import { PeopleMerges } from './PeopleMerges';
import { ProvidersTab } from './ProvidersTab';
import { DictionariesTab } from './DictionariesTab';
import { OcrTab } from './OcrTab';
import { EmbeddingsTab } from './EmbeddingsTab';
import { StorageTab } from './StorageTab';

let view;
let failing;
let failure;
beforeEach(() => {
  vi.clearAllMocks();
  failing = true;
  failure = new Error('fora do ar');
});
afterEach(() => view?.unmount());

// What each tab asks for, and the least of an answer that it can draw. A request fails while `failing` is set.
function serve(payload) {
  api.get.mockImplementation(async () => {
    if (failing) throw failure;
    return { data: payload };
  });
}
const alerts = () => [...document.body.querySelectorAll('[role="alert"]')];
const retryButtons = () => [...document.body.querySelectorAll('button')].filter((b) => b.textContent === 'Tentar de novo');

const CASES = [
  ['os trabalhos', () => <JobsTab />, 'Não foi possível carregar os trabalhos.', { data: [], total: 0 }],
  ['as contas', () => <AccountsTab isOwner />, 'Não foi possível carregar as contas.', { data: [] }],
  ['a lixeira', () => <TrashTab isOwner />, 'Não foi possível carregar a lixeira.', { data: [], policy: { enabled: false, days: 30 } }],
  ['as sugestões de duplicatas', () => <DuplicatesTab />, 'Não foi possível carregar as sugestões.', { data: [] }],
  ['a fila de sugestões', () => <SuggestionsTab />, 'Não foi possível carregar a fila.', { data: [] }],
  ['as sugestões de pessoas', () => <PeopleMerges />, 'Não foi possível carregar as sugestões.', { data: [] }],
  ['os provedores', () => <ProvidersTab isOwner />, 'Não foi possível carregar os provedores.', { data: [] }],
  ['os dicionários', () => <DictionariesTab isOwner />, 'Não foi possível carregar os dicionários.', { packages: [] }],
  ['as pastas autorizadas', () => <StorageTab isOwner />, 'Não foi possível carregar as pastas autorizadas.', { roots: [], data: [] }],
  ['os arquivos órfãos', () => <StorageTab isOwner />, 'Não foi possível carregar os arquivos órfãos.', { roots: [], data: [] }],
  ['o estado do backup', () => <StorageTab isOwner />, 'Não foi possível carregar o estado do backup.', { roots: [], data: [] }],
  ['a lista do OCR', () => <OcrTab isOwner />, 'Não foi possível carregar a lista.', { enabled: true, available: true, language: 'por', data: [] }],
  ['a configuração do OCR', () => <OcrTab isOwner />, 'Não foi possível carregar a configuração do OCR.', { enabled: true, available: true, language: 'por', data: [] }],
  ['a configuração da IA local', () => <EmbeddingsTab isOwner />, 'Não foi possível carregar a configuração.', { enabled: false, model: 'labse', available: true, models: [] }],
];

describe('the admin tabs that cannot load what they show', () => {
  it.each(CASES)('says it could not load %s, and offers to try again', async (_name, element, message, payload) => {
    serve(payload);
    view = await mount(element());
    expect(alerts().some((a) => a.textContent.includes(message)), message).toBe(true);
    expect(retryButtons().length).toBeGreaterThan(0);
  });

  it.each(CASES)('asks again when it is told to, and the message goes with %s', async (_name, element, message, payload) => {
    serve(payload);
    view = await mount(element());
    const before = api.get.mock.calls.length;
    failing = false;
    const said = () => alerts().filter((a) => a.textContent.includes(message)).length; // a tab can say the same twice, for two things
    const owner = alerts().find((a) => a.textContent.includes(message));
    const count = said();
    await view.click([...owner.querySelectorAll('button')].find((b) => b.textContent === 'Tentar de novo'));
    expect(api.get.mock.calls.length).toBeGreaterThan(before);
    expect(said(), `${message} went`).toBe(count - 1);
  });

  it('says nothing is empty when it could not tell: no list says "none" over an error', async () => {
    serve({ roots: [], data: [] });
    view = await mount(<StorageTab isOwner />);
    const text = document.body.textContent;
    expect(text).not.toContain('Nenhuma pasta autorizada.');
    expect(text).not.toContain('Nenhum arquivo órfão.');
    expect(text).not.toContain('Nenhum backup registrado');
  });
});

describe('and when the server says the person may not see it', () => {
  it.each(CASES)('says so, and does not offer to try again, for %s', async (_name, element, message, payload) => {
    failure = { response: { status: 403, data: 'Forbidden' } };
    serve(payload);
    view = await mount(element());
    const notes = [...document.body.querySelectorAll('[role="note"]')].map((n) => n.textContent);
    expect(notes.some((n) => n.includes('Você não tem permissão para ver isto')), message).toBe(true);
    expect([...document.body.querySelectorAll('[role="alert"]')].some((a) => a.textContent.includes(message)), `${message} is not said as a failure`).toBe(false);
  });
});
