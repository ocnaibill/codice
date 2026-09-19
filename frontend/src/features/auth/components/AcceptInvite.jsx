import React, { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { AuthCard } from './AuthCard';
import { FormField } from './FormField';

const MIN_PASSWORD = 8;

/** The page a person lands on from an invitation link: choose a name and a password. */
export function AcceptInvite({ token, onAccepted, onCancel }) {
  const [invite, setInvite] = useState(null); // null while checking, false when not valid
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api.get('/auth/invitation', { params: { token } })
      .then((res) => !cancelled && setInvite(res.data))
      .catch(() => !cancelled && setInvite(false));
    return () => { cancelled = true; };
  }, [token]);

  const submit = async (event) => {
    event.preventDefault();
    setError('');
    if (password.length < MIN_PASSWORD) return setError(`A senha precisa ter pelo menos ${MIN_PASSWORD} caracteres.`);
    if (password !== confirm) return setError('As senhas não coincidem.');
    setLoading(true);
    try {
      const res = await api.post('/auth/redeem', { token, username: username.trim(), password, email: email.trim() });
      localStorage.setItem('codice_token', res.data.token);
      onAccepted();
    } catch (err) {
      const data = err.response?.data;
      setError(err.response?.status === 404
        ? 'Este convite não vale mais. Peça um novo a quem o enviou.'
        : (typeof data === 'string' && data.trim()) || 'Não foi possível criar a conta.');
    } finally {
      setLoading(false);
    }
  };

  if (invite === null) {
    return <AuthCard title="Convite" subtitle="Verificando o convite…" />;
  }
  if (invite === false) {
    return (
      <AuthCard title="Convite inválido" subtitle="Este link foi usado, expirou ou foi revogado.">
        <p className="mb-4 text-center font-body text-[13px] text-ink-soft">Peça um novo convite a quem administra este acervo.</p>
        <button onClick={onCancel} className="w-full rounded-md bg-surface-alt py-2.5 font-body text-sm text-ink hover:brightness-95">
          Ir para o login
        </button>
      </AuthCard>
    );
  }

  return (
    <AuthCard title="Você foi convidado" subtitle={invite.role === 'admin' ? 'Crie sua conta de administrador' : 'Crie sua conta para acessar o acervo'}>
      {error && (
        <div role="alert" className="mb-4 rounded-md border border-red-200 bg-red-50 p-3 font-body text-[13px] text-red-700">{error}</div>
      )}
      <form onSubmit={submit} className="flex flex-col gap-4">
        <FormField label="Usuário" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="Como você quer ser chamado" required autoFocus />
        {!invite.emailRestricted && (
          <FormField label="Email (opcional)" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="voce@exemplo.com" />
        )}
        <FormField label="Senha" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Pelo menos 8 caracteres" required />
        <FormField label="Confirmar senha" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder="Repita a senha" required />
        <button type="submit" disabled={loading}
          className="mt-2 w-full rounded-md bg-brand py-2.5 font-body text-sm font-medium text-white shadow-[0px_1px_1.5px_rgba(0,0,0,0.1)] hover:brightness-110 disabled:opacity-50">
          {loading ? 'Criando…' : 'Criar conta'}
        </button>
      </form>
    </AuthCard>
  );
}
