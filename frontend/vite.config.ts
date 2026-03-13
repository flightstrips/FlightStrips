import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'
import tailwindcss from "@tailwindcss/vite";
import mdx from "@mdx-js/rollup"

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), mdx()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src')
    }
  },
  server: {
    host: '127.0.0.1',
    port: 8080
  }
})
