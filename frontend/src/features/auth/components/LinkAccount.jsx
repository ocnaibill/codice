import React, { useState } from 'react';
import { api } from '../../../lib/api';
import { AuthCard } from './AuthCard';
import { FormField } from './FormField';

/**
 * Second step of joining a directory identity to an existing local account. The
 * person already proved the directory password; the account is joined only once
 * they also prove the local one. The ticket is single-use and expires in minutes.
 */
export function LinkAccount({ ticket, username, onLinked, onBack }) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const submit = async (event) => {
    event.preventDefault();
    setError('');
    setLoading(true);
    try {
      const res = await api.post('/auth/link', { ticket, password });
      localStorage.setItem('codice_token', res.data.token);
      onLinked();
    } catch {
      setError('Senha local incorreta, ou o tempo acabou. Volte e entre de novo.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthCard title="Juntar as contas" subtitle={`Já existe uma conta local chamada ${username}`}>
      <p className="mb-4 font-body text-[13px] leading-relaxed text-ink-soft">
        Você entrou com a senha do diretório. Para ligar essa identidade à conta local {username}, e manter suas notas e seu progresso,
        confirme também a senha local. Depois disso você entra sempre pela senha do diretório.
      </p>
      {error && <div role="alert" className="mb-4 rounded-md border border-red-200 bg-red-50 p-3 font-body text-[13px] text-red-700">{error}</div>}
      <form onSubmit={submit} className="flex flex-col gap-4">
        <FormField label="Senha local" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="A senha da conta local" required autoFocus />
        <button type="submit" disabled={loading}
          className="w-full rounded-md bg-brand py-2.5 font-body text-sm font-medium text-white hover:brightness-110 disabled:opacity-50">
          {loading ? 'Juntando…' : 'Juntar as contas'}
        </button>
        <button type="button" onClick={onBack} className="font-body text-[13px] text-ink-soft hover:underline">Voltar ao login</button>
      </form>
    </AuthCard>
  );
}
