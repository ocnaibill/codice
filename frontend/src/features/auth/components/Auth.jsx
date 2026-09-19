import React, { useState } from 'react';
import { api } from '../../../lib/api';
import { AuthCard } from './AuthCard';
import { FormField } from './FormField';
import { ForgotPassword } from './ForgotPassword';

export function Auth({ onLoginSuccess }) {
  const [isLogin, setIsLogin] = useState(true);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [loading, setLoading] = useState(false);
  const [forgot, setForgot] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setSuccess('');
    setLoading(true);

    try {
      if (isLogin) {
        const res = await api.post('/auth/login', { username, password });
        localStorage.setItem('codice_token', res.data.token);
        onLoginSuccess();
      } else {
        await api.post('/auth/register', { username, email, password });
        setIsLogin(true);
        setSuccess('Conta criada com sucesso! Faça login para continuar.');
      }
    } catch (err) {
      setError(err.response?.data || 'Falha na autenticação. Verifique suas credenciais.');
    } finally {
      setLoading(false);
    }
  };

  if (forgot) {
    return <ForgotPassword initialUsername={username} onBack={() => setForgot(false)} />;
  }

  return (
    <AuthCard title={isLogin ? 'Bem-vindo de volta' : 'Crie sua conta'} subtitle={isLogin ? 'Entre para acessar seu acervo' : 'Comece a organizar sua biblioteca'}>
      {error && (
        <div className="mb-4 rounded-md border border-red-200 bg-red-50 p-3 font-body text-[13px] text-red-700">
          {error}
        </div>
      )}
      {success && (
        <div className="mb-4 rounded-md border border-success/30 bg-success/10 p-3 font-body text-[13px] text-success">
          {success}
        </div>
      )}

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <FormField
          label="Usuário"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder="Seu usuário"
          required
          autoFocus
        />

        {!isLogin && (
          <FormField
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="voce@exemplo.com"
            required
          />
        )}

        <FormField
          label="Senha"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="••••••••"
          required
        />

        {isLogin && (
          <button type="button" onClick={() => setForgot(true)} className="self-end font-body text-[12px] text-brand hover:underline">
            Esqueci minha senha
          </button>
        )}

        <button
          type="submit"
          disabled={loading}
          className="mt-2 w-full rounded-md bg-brand py-2.5 font-body text-sm font-medium text-white shadow-[0px_1px_1.5px_rgba(0,0,0,0.1)] transition-all hover:brightness-110 disabled:opacity-50"
        >
          {loading ? 'Processando...' : isLogin ? 'Entrar' : 'Criar conta'}
        </button>
      </form>

      <p className="mt-6 text-center font-body text-[13px] text-ink-soft">
        {isLogin ? 'Ainda não tem uma conta?' : 'Já tem uma conta?'}{' '}
        <button
          type="button"
          onClick={() => {
            setIsLogin(!isLogin);
            setError('');
            setSuccess('');
          }}
          className="ml-1 font-medium text-brand hover:underline"
        >
          {isLogin ? 'Criar conta' : 'Entrar'}
        </button>
      </p>
    </AuthCard>
  );
}
