import React, { useState } from 'react';
import { api } from '../../../lib/api';
import { AuthCard } from './AuthCard';
import { FormField } from './FormField';

/**
 * Asks whoever administers the library to reset a password. The answer is the same
 * whether or not the account exists, so this page never says which accounts do.
 */
export function ForgotPassword({ initialUsername = '', onBack }) {
  const [username, setUsername] = useState(initialUsername);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const submit = async (event) => {
    event.preventDefault();
    setError('');
    setLoading(true);
    try {
      await api.post('/auth/reset-request', { username: username.trim() });
      setSent(true);
    } catch (err) {
      setError(err.response?.status === 429 ? 'Muitas tentativas. Espere um pouco e tente de novo.' : 'Não foi possível enviar o pedido.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthCard title="Esqueci minha senha" subtitle="Peça a quem administra o acervo para redefinir">
      {sent ? (
        <>
          <p role="status" className="mb-4 rounded-md border border-success/30 bg-success/10 p-3 font-body text-[13px] text-success">
            Se a conta existir, o pedido foi enviado a quem administra este acervo. Ela ou ele vai confirmar que é você e
            entregar um link para escolher a nova senha. Combine isso por um meio de vocês, como uma mensagem ou pessoalmente.
          </p>
          <button onClick={onBack} className="w-full rounded-md bg-surface-alt py-2.5 font-body text-sm text-ink hover:brightness-95">Voltar ao login</button>
        </>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          {error && <div role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 font-body text-[13px] text-red-700">{error}</div>}
          <FormField label="Usuário" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="Seu usuário" required autoFocus />
          <button type="submit" disabled={loading}
            className="w-full rounded-md bg-brand py-2.5 font-body text-sm font-medium text-white hover:brightness-110 disabled:opacity-50">
            {loading ? 'Enviando…' : 'Pedir redefinição'}
          </button>
          <button type="button" onClick={onBack} className="font-body text-[13px] text-ink-soft hover:underline">Voltar ao login</button>
        </form>
      )}
    </AuthCard>
  );
}
