import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/auth': 'http://localhost:8081',
      '/user': 'http://localhost:8081',
      '/friends': 'http://localhost:8081',
      '/group': 'http://localhost:8081',
      '/session': 'http://localhost:8081',
      '/message': 'http://localhost:8081',
      '/ws': {
        target: 'ws://localhost:8081',
        ws: true
      }
    }
  }
})
