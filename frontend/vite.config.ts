import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Vite config. Порт 5173 совпадает с топологией compose (docs/02 §7).
// Прокси /api → gateway удобен в dev (контейнерное имя gateway:8080).
export default defineConfig({
  plugins: [react()],
  server: {
    host: "0.0.0.0",
    port: 5173,
    // В dev внутри docker-сети ходим к gateway по сервисному имени.
    proxy: {
      "/api": {
        target: process.env.VITE_GATEWAY_URL ?? "http://gateway:8080",
        changeOrigin: true,
      },
    },
  },
});
