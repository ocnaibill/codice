import { useRef, useState } from 'react';
import { useDialog } from '../../lib/useDialog';
import { MAX_DISPLAY_NAME, useSetDisplayName } from '../../features/auth/api/usePreferences';

/**
 * Asked once, at the first sign-in after it exists (#179): "como você quer ser chamado?". The answer is the name of the
 * greeting of the home ("Bom dia, Ana"); the user name is only what is typed to sign in. Closing the question, with Escape or
 * with the button, is an answer too: the user name stays, and it can be changed in Preferências.
 */
function DisplayNamePrompt({ username }) {
  const [name, setName] = useState('');
  const save = useSetDisplayName();
  const dialogRef = useRef(null);
  const keepUsername = () => save.mutate('');
  useDialog(dialogRef, { onEscape: keepUsername });

  const submit = (event) => {
    event.preventDefault();
    save.mutate(name);
  };

  return (
    <div className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4">
      <form
        ref={dialogRef}
        onSubmit={submit}
        role="dialog"
        aria-modal="true"
        aria-label="Como você quer ser chamado?"
        className="w-full max-w-md animate-pop-in rounded-xl bg-white p-6 shadow-2xl"
      >
        <h2 className="font-display text-xl font-semibold text-ink">Como você quer ser chamado?</h2>
        <p className="mt-2 text-[14px] leading-relaxed text-ink-soft">
          É o nome que aparece na sua saudação. Seu usuário (<strong className="text-ink">{username}</strong>) continua sendo o que você digita para entrar.
        </p>
        <label className="mt-4 flex flex-col gap-1 text-[13px] text-ink-soft">
          Seu nome
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoFocus
            autoComplete="given-name"
            maxLength={MAX_DISPLAY_NAME}
            placeholder={username}
            className="rounded-lg border border-border-hairline bg-surface px-3 py-2 text-[15px] text-ink outline-none focus:border-brand"
          />
        </label>
        {save.isError && <p role="alert" className="mt-3 text-[13px] text-danger">Não foi possível salvar agora.</p>}
        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <button type="button" onClick={keepUsername} disabled={save.isPending} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95 disabled:opacity-50">
            Usar meu usuário
          </button>
          <button type="submit" disabled={save.isPending || name.trim() === ''} className="rounded-lg bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110 disabled:opacity-50">
            Salvar
          </button>
        </div>
      </form>
    </div>
  );
}

/** The question, when it is the time of it: the account has not been asked yet (a server that does not say leaves it alone). */
export function AskDisplayName({ me }) {
  return me && me.displayNameAsked === false ? <DisplayNamePrompt username={me.username} /> : null;
}
