import React, { act, useState } from 'react';
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
import { ConfirmDialog } from './features/admin/components/ConfirmDialog';
import { FinishWorkPrompt } from './features/reader/components/FinishWorkPrompt';
import { EquivalentPositionPrompt } from './features/reader/components/EquivalentPositionPrompt';
import { ChangePasswordModal } from './components/layout/ChangePasswordModal';
import { PreferencesModal } from './components/layout/PreferencesModal';

// The dialogs of the app, as a person at the keyboard meets them: where the focus starts, where Tab can go, what Escape closes
// and where the focus goes back to. (The hook itself is tested in lib/useDialog.test.jsx; this is that the dialogs use it.)
let view;
afterEach(() => view?.unmount());

const $ = (id) => document.getElementById(id);
const dialogs = () => [...document.body.querySelectorAll('[role="dialog"]')];
const key = (name, options = {}) => act(async () => {
  (document.activeElement ?? document.body).dispatchEvent(new KeyboardEvent('keydown', { key: name, bubbles: true, cancelable: true, ...options }));
});
const labelOf = (el) => el?.getAttribute('aria-label') || el?.textContent?.trim();

describe('ConfirmDialog', () => {
  function Host({ onCancel, onChoose }) {
    const [open, setOpen] = useState(false);
    return (
      <div>
        <button id="opener" onClick={() => setOpen(true)}>Excluir</button>
        {open && (
          <ConfirmDialog
            title="Excluir a conta?"
            message={<p>Isso não se desfaz.</p>}
            choices={[{ label: 'Excluir conta', value: true, tone: 'danger' }]}
            onChoose={onChoose}
            onCancel={() => { onCancel(); setOpen(false); }}
          />
        )}
      </div>
    );
  }

  it('starts on the careful answer, keeps Tab inside, closes with Escape and gives the focus back', async () => {
    const onCancel = vi.fn();
    view = await mount(<Host onCancel={onCancel} onChoose={vi.fn()} />);
    $('opener').focus();
    await view.click($('opener'));
    expect(labelOf(document.activeElement)).toBe('Cancelar');
    // Before the first goes to the last, and after the last goes to the first (the ones in the middle are the browser's).
    await key('Tab', { shiftKey: true });
    expect(labelOf(document.activeElement)).toBe('Excluir conta');
    await key('Tab');
    expect(labelOf(document.activeElement)).toBe('Cancelar');
    await key('Escape');
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(dialogs()).toHaveLength(0);
    expect(document.activeElement).toBe($('opener'));
  });
});

describe('the prompts of the reader', () => {
  const others = [{ file: { format: 'pdf', percentComplete: 42 }, edition: { language: 'pt' } }];

  it('"Obra finalizada?" starts on "Não" (an Enter by habit marks nothing) and Escape is "Não"', async () => {
    const onKeep = vi.fn();
    const onFinish = vi.fn();
    view = await mount(<FinishWorkPrompt finished="Terminou." others={others} busy={false} onFinish={onFinish} onKeep={onKeep} />);
    expect(labelOf(document.activeElement)).toBe('Não, continuar a outra versão depois');
    await key('Escape');
    expect(onKeep).toHaveBeenCalledTimes(1);
    expect(onFinish).not.toHaveBeenCalled();
  });

  it('"Continuar de onde parou?" starts on declining, and Escape declines', async () => {
    const onDecline = vi.fn();
    const onAccept = vi.fn();
    view = await mount(
      <EquivalentPositionPrompt
        from={{ format: 'pdf', language: 'pt', percentComplete: 42 }} sourceExcerpt="x" status="found"
        candidates={[{ method: 'text', confidence: 'high', section: 'Cap. 3', excerpt: 'Trecho.', locator: { type: 'epub', href: 'c.xhtml' } }]}
        busy={false} onAccept={onAccept} onDecline={onDecline}
      />,
    );
    expect(labelOf(document.activeElement)).toBe('Não, abrir minha posição');
    await key('Escape');
    expect(onDecline).toHaveBeenCalledTimes(1);
    expect(onAccept).not.toHaveBeenCalled();
  });
});

describe('the dialogs of the account', () => {
  it('"Alterar senha" closes with Escape (it did not), starting on the first field', async () => {
    const onClose = vi.fn();
    view = await mount(<ChangePasswordModal onClose={onClose} />);
    expect(document.activeElement.type).toBe('password');
    await key('Escape');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('with one over the other, Escape closes only the one on top', async () => {
    const closeUnder = vi.fn();
    const closeOver = vi.fn();
    view = await mount(<div><PreferencesModal onClose={closeUnder} /><ChangePasswordModal onClose={closeOver} /></div>);
    expect(dialogs()).toHaveLength(2);
    await key('Escape');
    expect(closeOver).toHaveBeenCalledTimes(1);
    expect(closeUnder).not.toHaveBeenCalled();
  });

  it('Tab does not leave the one on top for the one under it', async () => {
    view = await mount(<div><PreferencesModal onClose={vi.fn()} /><ChangePasswordModal onClose={vi.fn()} /></div>);
    const over = dialogs()[1];
    for (let i = 0; i < 8; i += 1) {
      await key('Tab');
      expect(over.contains(document.activeElement), `after ${i + 1} Tabs`).toBe(true);
    }
  });
});
