import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev
export default defineConfig({
  envDir: '../', // Load .env from the project root
  plugins: [
    react(),
    tailwindcss(),
  ],
  // Point the dev UI at an existing local Códice installation without changing
  // its CORS policy or exposing a second API origin to the browser.
  server: process.env.CODICE_DEV_API_TARGET ? {
    proxy: {
      '^/(auth|works|stats|favorites|notes|search|progress|files|covers|ws|admin|users|invitations|password-resets|upload|ownership|metadata|healthz)(/|\\?|$)': {
        target: process.env.CODICE_DEV_API_TARGET,
        changeOrigin: true,
        ws: true,
      },
    },
  } : undefined,
})
