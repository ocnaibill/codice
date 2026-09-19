import { useState } from 'react';
import { api } from '../../lib/api';

const MIN_PASSWORD = 8;

/** Lets a person change their own password; other devices are signed out. */
export function ChangePasswordModal({ onClose }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  const submit = async (event) => {
    event.preventDefault();
    setError('');
    if (next.length < MIN_PASSWORD) return setError(`A nova senha precisa ter pelo menos ${MIN_PASSWORD} caracteres.`);
    if (next !== confirm) return setError('As senhas novas não coincidem.');
    setBusy(true);
    try {
      await api.post('/auth/password', { current, new: next });
      setDone(true);
    } catch (err) {
      const data = err.response?.data;
      setError((typeof data === 'string' && data.trim()) || 'Não foi possível alterar a senha.');
    } finally {
      setBusy(false);
    }
  };

  const field = (label, value, set, autoFocus) => (
    <label className="flex flex-col gap-1 text-[11px] uppercase tracking-wide text-ink-soft">
      {label}
      <input type="password" value={value} onChange={(event) => set(event.target.value)} autoFocus={autoFocus}
        autoComplete={label === 'Senha atual' ? 'current-password' : 'new-password'} required
        className="rounded bg-surface px-3 py-2 text-[14px] normal-case text-ink outline-none" />
    </label>
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}>
      <div role="dialog" aria-modal="true" aria-label="Alterar senha" className="w-full max-w-sm rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Alterar senha</h2>
        {done ? (
          <>
            <p role="status" className="mt-3 text-[14px] text-ink-soft">Senha alterada. Os outros aparelhos em que você estava conectado foram desconectados.</p>
            <div className="mt-5 flex justify-end">
              <button onClick={onClose} className="rounded bg-brand px-4 py-2 text-[13px] text-white hover:brightness-110">Fechar</button>
            </div>
          </>
        ) : (
          <form onSubmit={submit} className="mt-4 flex flex-col gap-3">
            {error && <p role="alert" className="text-[13px] text-red-700">{error}</p>}
            {field('Senha atual', current, setCurrent, true)}
            {field('Nova senha', next, setNext)}
            {field('Confirmar nova senha', confirm, setConfirm)}
            <div className="mt-2 flex justify-end gap-2">
              <button type="button" onClick={onClose} className="rounded px-4 py-2 text-[13px] text-ink-soft hover:text-ink">Cancelar</button>
              <button type="submit" disabled={busy} className="rounded bg-brand px-4 py-2 text-[13px] text-white hover:brightness-110 disabled:opacity-50">
                {busy ? 'Alterando…' : 'Alterar senha'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
