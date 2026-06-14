import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Vite config (PsyNote frontend).
// Порт 5173 совпадает с топологией compose (docs/02 §7); хост-порт 5174.
// Прокси /api → gateway удобен в dev: фронт ходит на относительный /api,
// секреты и реальный адрес gateway остаются на стороне прокси (docs/07 §1).
// VITE_GATEWAY_URL — адрес gateway в docker-сети (gateway:8080) или localhost.
export default defineConfig({
  plugins: [react()],
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_GATEWAY_URL ?? "http://gateway:8080",
        changeOrigin: true,
      },
    },
  },
});
