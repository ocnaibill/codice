import React, { useState } from 'react';
import { api } from '../../../lib/api';

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
      setError('Passwords do not match.');
      return;
    }

    if (password.length < 6) {
      setError('Password must be at least 6 characters long.');
      return;
    }

    setLoading(true);

    try {
      const res = await api.post('/auth/setup', { username, email, password });
      localStorage.setItem('codice_token', res.data.token);
      onSetupComplete();
    } catch (err) {
      setError(err.response?.data || 'Failed to complete initial setup.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex h-screen items-center justify-center bg-zinc-950 p-4">
      <div className="w-full max-w-lg p-8 bg-zinc-900 border border-zinc-800 rounded-xl shadow-2xl">
        <div className="text-center mb-8">
          <span className="text-4xl mb-2 block">📚</span>
          <h1 className="text-3xl font-semibold text-zinc-100 tracking-tight">Welcome to Códice</h1>
          <p className="text-zinc-400 text-sm mt-1">
            First-time setup — Create Master Administrator account
          </p>
        </div>

        {/* Feature Badges */}
        <div className="grid grid-cols-3 gap-2 mb-6 text-center text-[11px] font-mono text-zinc-400">
          <div className="bg-zinc-950 p-2 rounded border border-zinc-800">
            🔒 Self-Hosted
          </div>
          <div className="bg-zinc-950 p-2 rounded border border-zinc-800">
            🚀 Auto Scraper
          </div>
          <div className="bg-zinc-950 p-2 rounded border border-zinc-800">
            📖 Universal Reader
          </div>
        </div>

        {error && (
          <div className="p-3 mb-4 text-xs font-medium text-red-400 bg-red-950/40 border border-red-900/60 rounded-md">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Admin Username</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-4 py-2.5 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 transition-colors"
              required
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Admin Email</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-4 py-2.5 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 transition-colors"
              required
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Master Password</label>
            <input
              type="password"
              placeholder="••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-4 py-2.5 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 transition-colors"
              required
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Confirm Master Password</label>
            <input
              type="password"
              placeholder="••••••••"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-4 py-2.5 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 transition-colors"
              required
            />
          </div>

          <button 
            type="submit" 
            disabled={loading}
            className="w-full py-3 mt-4 bg-blue-600 text-white font-medium text-sm rounded-md hover:bg-blue-500 transition-colors disabled:opacity-50 shadow-lg shadow-blue-900/20"
          >
            {loading ? 'Initializing Códice...' : 'Complete Initial Setup'}
          </button>
        </form>
      </div>
    </div>
  );
}
