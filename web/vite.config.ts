import path from "node:path"
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

// Dev: `npm run dev` proxies the API to the Go server on :8080.
// Build: output goes to web/dist, which the Go binary embeds.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/audio": "http://localhost:8080",
    },
  },
})
