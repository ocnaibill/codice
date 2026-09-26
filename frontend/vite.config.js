import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev
export default defineConfig(({ command }) => {
  // In local proxy mode, make browser requests relative even if the developer's
  // root .env normally sets VITE_API_URL for another installation.
  const devApiTarget = command === 'serve' ? process.env.CODICE_DEV_API_TARGET : undefined
  if (devApiTarget) process.env.VITE_API_URL = ''

  return {
    envDir: '../', // Load .env from the project root
    plugins: [
      react(),
      tailwindcss(),
    ],
    // Point the dev UI at an existing local Códice installation without changing
    // its CORS policy or exposing a second API origin to the browser.
    server: devApiTarget ? {
      proxy: {
        '^/(auth|works|stats|favorites|notes|search|progress|files|covers|ws|admin|users|invitations|password-resets|upload|ownership|metadata|healthz)(/|\\?|$)': {
          target: devApiTarget,
          changeOrigin: true,
          ws: true,
        },
      },
    } : undefined,
  }
})
