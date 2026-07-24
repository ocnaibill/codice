import axios from 'axios';

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || 'http://localhost:8080',
  timeout: 10000,
});

// Espaço reservado para injetar token de autenticação no futuro
api.interceptors.request.use((config) => {
  // const token = localStorage.getItem('codice_token');
  // if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});