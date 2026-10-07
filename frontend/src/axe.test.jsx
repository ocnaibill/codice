import React, { act } from 'react';
import axe from 'axe-core';
import { describe, it, expect, afterEach, vi } from 'vitest';

vi.mock('./lib/api', () => ({ api: { get: vi.fn(async () => ({ data: {} })), post: vi.fn() } }));
vi.mock('./features/auth/api/usePreferences', () => ({
  NAME_ORDERS: { given_first: { label: 'Nome primeiro', example: 'Frank Herbert' }, family_first: { label: 'Sobrenome primeiro', example: 'Herbert, Frank' } },
  usePreferences: () => ({ data: { choice: '', library: 'given_first' }, isLoading: false, isError: false }),
  useSetNameOrder: () => ({ mutate: vi.fn(), isPending: false }),
  useSetDisplayName: () => ({ mutate: vi.fn(), isPending: false, isError: false }),
  MAX_DISPLAY_NAME: 60,
}));
vi.mock('./features/auth/components/MyData', () => ({ MyData: () => null }));

import { mount } from './features/admin/testUtils';
import { Auth } from './features/auth/components/Auth';
import { AuthCard } from './features/auth/components/AuthCard';
import { ToastRegion } from './components/ui/ToastRegion';
import { toast, useToasts } from './components/ui/toast';
import { GreetingStats } from './features/home/components/GreetingStats';
import { ConfirmDialog } from './features/admin/components/ConfirmDialog';
import { FinishWorkPrompt } from './features/reader/components/FinishWorkPrompt';
import { EquivalentPositionPrompt } from './features/reader/components/EquivalentPositionPrompt';
import { ChangePasswordModal } from './components/layout/ChangePasswordModal';
import { PreferencesModal } from './components/layout/PreferencesModal';
import ReadingSettingsPanel from './features/reader/components/viewers/ReadingSettingsPanel';
import { DEFAULT_SETTINGS } from './features/reader/epubThemes';

// What axe (the rules of WCAG 2.2 AA and the good practices) finds on what is rendered, for the pieces that are cheap to render
// here: names of the controls, roles that are allowed, the structure of lists, landmarks. It runs in jsdom, which has no layout:
// the contrast is measured by the tests of the theme (accessibility.test.js) and by axe in the real browser.
// `region` is left out: it is about a page, and a piece is not one.
async function audit(element) {
  const result = await axe.run(element, {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
    rules: { 'color-contrast': { enabled: false }, region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
  });
  return result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 90)).join(' | ')}`);
}

let view;
afterEach(() => {
  view?.unmount();
  useToasts.getState().clear();
});

describe('what axe finds', () => {
  it('on the sign-in screen: a page with its main landmark', async () => {
    view = await mount(<Auth onLoginSuccess={() => {}} />);
    expect(view.container.querySelectorAll('main')).toHaveLength(1);
    expect(await audit(document.body)).toEqual([]);
  });

  it('on a card of the first access, too', async () => {
    view = await mount(<AuthCard title="Bem-vindo"><p>texto</p></AuthCard>);
    expect(view.container.querySelectorAll('main')).toHaveLength(1);
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the region of notices: a landmark with a name, with the notices inside', async () => {
    view = await mount(<ToastRegion />);
    await act(async () => { toast.success('Salvo.'); });
    const region = document.body.querySelector('[aria-label="Avisos"]');
    expect(region.getAttribute('role')).toBe('region');
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the statistics of the home: a list of terms and descriptions, and nothing else in it', async () => {
    const stats = { worksTotal: 10, catalogedPercent: 100, inProgress: 1, finishedThisMonth: 0, readingMinutes: 0 };
    view = await mount(<GreetingStats userName="Ana" stats={stats} isLoading={false} error={null} onRetry={() => {}} />);
    const list = view.container.querySelector('dl');
    expect(list).not.toBeNull();
    for (const child of list.children) {
      // (an icon that is hidden from the assistive technologies is not content)
      for (const inner of [...child.children].filter((el) => el.getAttribute('aria-hidden') !== 'true')) expect(['DT', 'DD']).toContain(inner.tagName);
    }
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the dialogs that ask', async () => {
    view = await mount(
      <div>
        <ConfirmDialog title="Excluir?" message={<p>Isso não se desfaz.</p>} choices={[{ label: 'Excluir', value: true, tone: 'danger' }]} onChoose={() => {}} onCancel={() => {}} requireText="dono" />
      </div>,
    );
    expect(await audit(document.body)).toEqual([]);
    view.unmount();
    view = await mount(<FinishWorkPrompt finished="Terminou." others={[{ file: { format: 'pdf', percentComplete: 42 }, edition: { language: 'pt' } }]} busy={false} onFinish={() => {}} onKeep={() => {}} />);
    expect(await audit(document.body)).toEqual([]);
    view.unmount();
    view = await mount(
      <EquivalentPositionPrompt from={{ format: 'pdf', language: 'pt', percentComplete: 42 }} sourceExcerpt="x" status="ambiguous"
        candidates={[{ method: 'text', confidence: 'medium', section: 'A', excerpt: 'um', locator: {} }, { method: 'text', confidence: 'medium', section: 'B', excerpt: 'dois', locator: {} }]}
        busy={false} onAccept={() => {}} onDecline={() => {}} />,
    );
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the dialogs of the account', async () => {
    view = await mount(<ChangePasswordModal onClose={() => {}} />);
    expect(await audit(document.body)).toEqual([]);
    view.unmount();
    view = await mount(<PreferencesModal onClose={() => {}} />);
    expect(await audit(document.body)).toEqual([]);
  });

  it('on the panel of the look of the text', async () => {
    view = await mount(<ReadingSettingsPanel settings={DEFAULT_SETTINGS} onChange={() => {}} onClose={() => {}} />);
    expect(await audit(document.body)).toEqual([]);
  });

  it('and it does find what is wrong (the check is not empty): a button with no name, a list with a stray child', async () => {
    view = await mount(<div><button /><dl><div><dt>a</dt><dd>b</dd><p>c</p></div></dl></div>);
    const found = (await audit(document.body)).map((line) => line.split(':')[0]);
    expect(found).toEqual(expect.arrayContaining(['button-name', 'definition-list']));
  });
});
