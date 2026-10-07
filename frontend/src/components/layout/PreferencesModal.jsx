import { useRef, useState } from 'react';
import { useDialog } from '../../lib/useDialog';
import { LoadError } from '../ui/LoadError';
import { MAX_DISPLAY_NAME, NAME_ORDERS, usePreferences, useSetDisplayName, useSetNameOrder } from '../../features/auth/api/usePreferences';
import { useMe } from '../../features/auth/api/useMe';
import { getKeepScreenOn, saveKeepScreenOn } from '../../features/reader/preferences';
import { MyData } from '../../features/auth/components/MyData';

/**
 * How this account wants the names of authors shown, and how the library is sorted by author. It only
 * changes what is shown: what is stored is never touched (#64). "The library's" is the default the owner set.
 */
function CalledBy({ prefs }) {
  const { data: me } = useMe();
  const save = useSetDisplayName();
  const saved = prefs?.displayName ?? '';
  const [draft, setDraft] = useState(null); // null: what is saved is what is shown
  const shown = draft ?? saved;
  const changed = draft !== null && draft.trim() !== saved;
  return (
    <form
      className="mt-4"
      onSubmit={(event) => {
        event.preventDefault();
        save.mutate(draft ?? saved, { onSuccess: () => setDraft(null) });
      }}
    >
      <label className="flex flex-col gap-1 text-[13px] font-medium text-ink">
        Como você quer ser chamado
        <input
          value={shown}
          onChange={(event) => setDraft(event.target.value)}
          maxLength={MAX_DISPLAY_NAME}
          autoComplete="given-name"
          placeholder={me?.username ?? ''}
          className="rounded-lg border border-border-hairline bg-surface px-3 py-2 text-[14px] font-normal text-ink outline-none focus:border-brand"
        />
      </label>
      <p className="mt-1 text-[12px] text-ink-soft">
        É o nome da sua saudação. Vazio, vale o seu usuário{me?.username ? ` (${me.username})` : ''}, que continua sendo o que você digita para entrar.
      </p>
      <div className="mt-2 flex items-center gap-3">
        <button type="submit" disabled={!changed || save.isPending} className="rounded-lg bg-brand px-3 py-1.5 text-[13px] font-medium text-white hover:brightness-110 disabled:opacity-40">Salvar</button>
        {save.isError && <span role="alert" className="text-[13px] text-danger">Não foi possível salvar.</span>}
      </div>
    </form>
  );
}

/** Whether the screen stays on while this device reads (#180): about the battery of this phone, so it is of the device. */
function KeepScreenOn() {
  const [on, setOn] = useState(getKeepScreenOn);
  const supported = typeof navigator !== 'undefined' && !!navigator.wakeLock?.request;
  return (
    <div className="mt-4">
      <label className={`flex items-start gap-3 text-[14px] text-ink ${supported ? 'cursor-pointer' : 'opacity-60'}`}>
        <input
          type="checkbox"
          className="mt-1"
          checked={supported && on}
          disabled={!supported}
          onChange={(event) => { setOn(event.target.checked); saveKeepScreenOn(event.target.checked); }}
        />
        <span>
          Manter a tela acesa enquanto eu leio
          <span className="block text-[12px] text-ink-faint">
            {supported
              ? 'Só neste aparelho. A tela não escurece sozinha durante a leitura de livros, PDFs, quadrinhos e textos (o áudio não precisa).'
              : 'Este navegador, ou este endereço sem HTTPS, não permite. O aparelho escurece a tela como ele estiver configurado.'}
          </span>
        </span>
      </label>
    </div>
  );
}

export function PreferencesModal({ onClose }) {
  const { data: prefs, isLoading, isError, error, refetch, isRefetching } = usePreferences();
  const save = useSetNameOrder();

  const dialogRef = useRef(null);
  useDialog(dialogRef, { onEscape: onClose });

  const options = [
    ...Object.entries(NAME_ORDERS).map(([value, { label, example }]) => ({ value, label, hint: example })),
    { value: '', label: 'O padrão da biblioteca', hint: prefs ? `hoje: ${NAME_ORDERS[prefs.library]?.label ?? ''}` : '' },
  ];

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label="Preferências" className="max-h-[92vh] w-full max-w-md animate-pop-in overflow-y-auto rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Preferências</h2>
        {prefs && <CalledBy prefs={prefs} />}
        <fieldset className="mt-4" disabled={isLoading || save.isPending}>
          <legend className="text-[13px] font-medium text-ink">Como mostrar o nome dos autores</legend>
          <p className="mt-1 text-[12px] text-ink-soft">
            Muda só como o nome aparece e como o acervo se ordena por autor; nada do que está guardado é alterado.
          </p>
          {isError && <LoadError className="mt-3" error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar as preferências.</LoadError>}
          <div className="mt-3 flex flex-col gap-2">
            {options.map((option) => (
              <label key={option.value || 'library'} className="flex cursor-pointer items-start gap-3 rounded-lg border border-border-hairline p-3 text-[14px] text-ink has-[:checked]:border-brand">
                <input
                  type="radio"
                  name="name-order"
                  className="mt-1"
                  checked={(prefs?.choice ?? '') === option.value}
                  onChange={() => save.mutate(option.value)}
                />
                <span>
                  {option.label}
                  {option.hint && <span className="block text-[12px] text-ink-faint">{option.hint}</span>}
                </span>
              </label>
            ))}
          </div>
        </fieldset>
        {save.isError && <p role="alert" className="mt-3 text-[13px] text-danger">Não foi possível salvar.</p>}
        <KeepScreenOn />
        <MyData />
        <div className="mt-5 flex justify-end">
          <button onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Fechar</button>
        </div>
      </div>
    </div>
  );
}
