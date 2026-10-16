import path from "node:path"
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

// Dev: `npm run dev` proxies the API to the Go server on :8080, or to
// API_URL (e.g. API_URL=http://localhost:8090 for a second server).
// Build: output goes to web/dist, which the Go binary embeds.
const api = process.env.API_URL || "http://localhost:8080"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  server: {
    proxy: {
      "/api": api,
      "/audio": api,
    },
  },
})
