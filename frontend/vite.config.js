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
})
