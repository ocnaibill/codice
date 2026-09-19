import { useState } from 'react';
import { JobsTab } from './components/JobsTab';
import { StorageTab } from './components/StorageTab';
import { TrashTab } from './components/TrashTab';
import { DuplicatesTab } from './components/DuplicatesTab';
import { AccountsTab } from './components/AccountsTab';
import { LdapTab } from './components/LdapTab';

const TABS = [
  ['jobs', 'Trabalhos'],
  ['storage', 'Armazenamento'],
  ['trash', 'Lixeira'],
  ['duplicates', 'Duplicatas e OCR'],
  ['accounts', 'Contas'],
];

/** The administration area. Owner and admin see it; some actions are the owner's alone. */
export function AdminPage({ isOwner, onClose }) {
  const [tab, setTab] = useState('jobs');
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
        {[...TABS, ...(isOwner ? [['ldap', 'Login externo']] : [])].map(([key, label]) => (
          <button
            key={key}
            role="tab"
            aria-selected={tab === key}
            onClick={() => setTab(key)}
            className={`-mb-px border-b-2 px-4 py-2 text-[13px] ${
              tab === key ? 'border-brand text-brand' : 'border-transparent text-ink-soft hover:text-ink'
            }`}
          >
            {label}
          </button>
        ))}
      </div>
      <div className="mt-5">
        {tab === 'jobs' && <JobsTab />}
        {tab === 'storage' && <StorageTab isOwner={isOwner} />}
        {tab === 'trash' && <TrashTab isOwner={isOwner} />}
        {tab === 'duplicates' && <DuplicatesTab />}
        {tab === 'accounts' && <AccountsTab isOwner={isOwner} />}
        {tab === 'ldap' && isOwner && <LdapTab />}
      </div>
    </div>
  );
}
