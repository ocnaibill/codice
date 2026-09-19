import React, { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { AuthCard } from './AuthCard';
import { FormField } from './FormField';

const MIN_PASSWORD = 8;

/** The page a reset link opens: choose a new password. Every device is signed out afterwards. */
export function ResetPassword({ token, onDone }) {
  const [info, setInfo] = useState(null); // null while checking, false when not valid
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api.get('/auth/reset', { params: { token } })
      .then((res) => !cancelled && setInfo(res.data))
      .catch(() => !cancelled && setInfo(false));
    return () => { cancelled = true; };
  }, [token]);

  const submit = async (event) => {
    event.preventDefault();
    setError('');
    if (password.length < MIN_PASSWORD) return setError(`A senha precisa ter pelo menos ${MIN_PASSWORD} caracteres.`);
    if (password !== confirm) return setError('As senhas não coincidem.');
    setLoading(true);
    try {
      await api.post('/auth/reset', { token, password });
      setDone(true);
    } catch (err) {
      const data = err.response?.data;
      setError(err.response?.status === 404
        ? 'Este link não vale mais. Peça um novo a quem administra o acervo.'
        : (typeof data === 'string' && data.trim()) || 'Não foi possível alterar a senha.');
    } finally {
      setLoading(false);
    }
  };

  if (info === null) return <AuthCard title="Redefinir senha" subtitle="Verificando o link…" />;

  if (info === false) {
    return (
      <AuthCard title="Link inválido" subtitle="Este link foi usado, expirou ou não vale mais.">
        <p className="mb-4 text-center font-body text-[13px] text-ink-soft">Peça um novo pedido a quem administra este acervo.</p>
        <button onClick={onDone} className="w-full rounded-md bg-surface-alt py-2.5 font-body text-sm text-ink hover:brightness-95">Ir para o login</button>
      </AuthCard>
    );
  }

  if (done) {
    return (
      <AuthCard title="Senha alterada" subtitle="Agora é só entrar com a nova senha">
        <p role="status" className="mb-4 text-center font-body text-[13px] text-ink-soft">
          Você foi desconectado de todos os aparelhos e os tokens de aplicativo foram encerrados.
        </p>
        <button onClick={onDone} className="w-full rounded-md bg-brand py-2.5 font-body text-sm font-medium text-white hover:brightness-110">Ir para o login</button>
      </AuthCard>
    );
  }

  return (
    <AuthCard title="Redefinir senha" subtitle={`Escolha uma nova senha para ${info.username}`}>
      {error && <div role="alert" className="mb-4 rounded-md border border-red-200 bg-red-50 p-3 font-body text-[13px] text-red-700">{error}</div>}
      <form onSubmit={submit} className="flex flex-col gap-4">
        <FormField label="Nova senha" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Pelo menos 8 caracteres" required autoFocus />
        <FormField label="Confirmar senha" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder="Repita a senha" required />
        <button type="submit" disabled={loading}
          className="mt-2 w-full rounded-md bg-brand py-2.5 font-body text-sm font-medium text-white hover:brightness-110 disabled:opacity-50">
          {loading ? 'Alterando…' : 'Alterar senha'}
        </button>
      </form>
    </AuthCard>
  );
}
