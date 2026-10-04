import { useState } from 'react';
import { JobsTab } from './components/JobsTab';
import { StorageTab } from './components/StorageTab';
import { SystemTab } from './components/SystemTab';
import { TrashTab } from './components/TrashTab';
import { DuplicatesTab } from './components/DuplicatesTab';
import { AccountsTab } from './components/AccountsTab';
import { LdapTab } from './components/LdapTab';
import { EmbeddingsTab } from './components/EmbeddingsTab';
import { SuggestionsTab } from './components/SuggestionsTab';
import { ProvidersTab } from './components/ProvidersTab';
import { OcrTab } from './components/OcrTab';
import { DictionariesTab } from './components/DictionariesTab';
import { useSuggestionQueue } from './api/admin';

const TABS = [
  ['jobs', 'Trabalhos'],
  ['storage', 'Armazenamento'],
  ['system', 'Sistema'],
  ['trash', 'Lixeira'],
  ['suggestions', 'Sugestões'],
  ['providers', 'Provedores'],
  ['duplicates', 'Duplicatas'],
  ['ocr', 'OCR'],
  ['dictionaries', 'Dicionários'],
  ['accounts', 'Contas'],
];

/** The administration area. Owner and admin see it; some actions are the owner's alone. */
export function AdminPage({ isOwner, onClose }) {
  const [tab, setTab] = useState('jobs');
  const waiting = useSuggestionQueue().data?.total ?? 0;
  return (
    <div className="mx-auto max-w-5xl px-4 py-6 sm:px-6">
      <div className="flex items-center justify-between gap-3">
        <h1 className="font-display text-2xl font-semibold text-ink">Administração</h1>
        {onClose && (
          <button onClick={onClose} className="rounded bg-surface-alt px-3 py-1.5 text-[12px] text-ink hover:brightness-95">
            ← Voltar ao acervo
          </button>
        )}
      </div>
      <div role="tablist" className="mt-4 flex flex-wrap gap-1 border-b border-border-hairline">
        {[...TABS, ...(isOwner ? [['embeddings', 'IA local'], ['ldap', 'Login externo']] : [])].map(([key, label]) => (
          <button
            key={key}
            role="tab"
            aria-selected={tab === key}
            onClick={() => setTab(key)}
            className={`-mb-px border-b-2 px-4 py-2 text-[13px] ${
              tab === key ? 'border-brand text-brand' : 'border-transparent text-ink-soft hover:text-ink'
            }`}
          >
            {key === 'suggestions' && waiting > 0 ? `${label} (${waiting})` : label}
          </button>
        ))}
      </div>
      <div className="mt-5">
        {tab === 'jobs' && <JobsTab />}
        {tab === 'storage' && <StorageTab isOwner={isOwner} />}
        {tab === 'system' && <SystemTab isOwner={isOwner} />}
        {tab === 'trash' && <TrashTab isOwner={isOwner} />}
        {tab === 'suggestions' && <SuggestionsTab isOwner={isOwner} onOpenProviders={() => setTab('providers')} />}
        {tab === 'providers' && <ProvidersTab isOwner={isOwner} />}
        {tab === 'duplicates' && <DuplicatesTab />}
        {tab === 'ocr' && <OcrTab isOwner={isOwner} />}
        {tab === 'dictionaries' && <DictionariesTab isOwner={isOwner} />}
        {tab === 'accounts' && <AccountsTab isOwner={isOwner} />}
        {tab === 'ldap' && isOwner && <LdapTab />}
        {tab === 'embeddings' && isOwner && <EmbeddingsTab />}
      </div>
    </div>
  );
}
