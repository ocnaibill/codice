import { useState } from 'react';
import { JobsTab } from './components/JobsTab';
import { StorageTab } from './components/StorageTab';
import { TrashTab } from './components/TrashTab';
import { DuplicatesTab } from './components/DuplicatesTab';

const TABS = [
  ['jobs', 'Trabalhos'],
  ['storage', 'Armazenamento'],
  ['trash', 'Lixeira'],
  ['duplicates', 'Duplicatas e OCR'],
];

/** The administration area. Owner and admin see it; some actions are the owner's alone. */
export function AdminPage({ isOwner }) {
  const [tab, setTab] = useState('jobs');
  return (
    <div className="mx-auto max-w-5xl px-4 py-6 sm:px-6">
      <h1 className="font-display text-2xl font-semibold text-ink">Administração</h1>
      <div role="tablist" className="mt-4 flex flex-wrap gap-1 border-b border-border-hairline">
        {TABS.map(([key, label]) => (
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
      </div>
    </div>
  );
}
