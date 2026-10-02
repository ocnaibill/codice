import { useEffect } from 'react';
import { NAME_ORDERS, usePreferences, useSetNameOrder } from '../../features/auth/api/usePreferences';

/**
 * How this account wants the names of authors shown, and how the library is sorted by author. It only
 * changes what is shown: what is stored is never touched (#64). "The library's" is the default the owner set.
 */
export function PreferencesModal({ onClose }) {
  const { data: prefs, isLoading, isError } = usePreferences();
  const save = useSetNameOrder();

  useEffect(() => {
    const onKey = (event) => event.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const options = [
    ...Object.entries(NAME_ORDERS).map(([value, { label, example }]) => ({ value, label, hint: example })),
    { value: '', label: 'O padrão da biblioteca', hint: prefs ? `hoje: ${NAME_ORDERS[prefs.library]?.label ?? ''}` : '' },
  ];

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div role="dialog" aria-modal="true" aria-label="Preferências" className="w-full max-w-md animate-pop-in rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Preferências</h2>
        <fieldset className="mt-4" disabled={isLoading || save.isPending}>
          <legend className="text-[13px] font-medium text-ink">Como mostrar o nome dos autores</legend>
          <p className="mt-1 text-[12px] text-ink-soft">
            Muda só como o nome aparece e como o acervo se ordena por autor; nada do que está guardado é alterado.
          </p>
          {isError && <p className="mt-3 text-[13px] text-red-700">Não foi possível carregar as preferências.</p>}
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
        {save.isError && <p role="alert" className="mt-3 text-[13px] text-red-700">Não foi possível salvar.</p>}
        <div className="mt-5 flex justify-end">
          <button onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Fechar</button>
        </div>
      </div>
    </div>
  );
}
