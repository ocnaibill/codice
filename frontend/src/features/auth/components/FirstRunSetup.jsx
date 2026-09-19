import React, { useState } from 'react';
import { api } from '../../../lib/api';
import { AuthCard } from './AuthCard';
import { FormField } from './FormField';

const FEATURES = [
  { icon: '🔒', label: 'Auto-hospedado' },
  { icon: '🔎', label: 'Catalogação automática' },
  { icon: '📖', label: 'Leitor universal' },
];

export function FirstRunSetup({ onSetupComplete }) {
  const [username, setUsername] = useState('admin');
  const [email, setEmail] = useState('admin@codice.local');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');

    if (password !== confirmPassword) {
      setError('As senhas não coincidem.');
      return;
    }

    if (password.length < 6) {
      setError('A senha precisa ter pelo menos 6 caracteres.');
      return;
    }

    setLoading(true);

    try {
      const res = await api.post('/auth/setup', { username, email, password });
      localStorage.setItem('codice_token', res.data.token);
      onSetupComplete();
    } catch (err) {
      setError(err.response?.data || 'Falha ao concluir a configuração inicial.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthCard title="Bem-vindo ao Códice" subtitle="Configuração inicial — crie a conta de administrador">
      <div className="mb-6 grid grid-cols-3 gap-2">
        {FEATURES.map((f) => (
          <div key={f.label} className="flex flex-col items-center gap-1 rounded-md bg-surface px-2 py-3 text-center">
            <span className="text-lg leading-none">{f.icon}</span>
            <span className="font-body text-[10px] leading-tight text-ink-soft">{f.label}</span>
          </div>
        ))}
      </div>

      {error && (
        <div className="mb-4 rounded-md border border-red-200 bg-red-50 p-3 font-body text-[13px] text-red-700">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <FormField label="Usuário administrador" value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus />
        <FormField label="Email do administrador" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        <FormField label="Senha mestra" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" required />
        <FormField
          label="Confirmar senha mestra"
          type="password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          placeholder="••••••••"
          required
        />

        <button
          type="submit"
          disabled={loading}
          className="mt-2 w-full rounded-md bg-brand py-2.5 font-body text-sm font-medium text-white shadow-[0px_1px_1.5px_rgba(0,0,0,0.1)] transition-all hover:brightness-110 disabled:opacity-50"
        >
          {loading ? 'Inicializando o Códice...' : 'Concluir configuração inicial'}
        </button>
      </form>
    </AuthCard>
  );
}
