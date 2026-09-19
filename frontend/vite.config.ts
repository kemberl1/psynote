import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

// Метка сборки: вшивается в бандл и кладётся в dist/version.json.
// Открытая до деплоя вкладка сравнивает их перед генерацией (buildVersion.ts),
// чтобы не собирать брифы дневников по старым правилам.
const BUILD_ID = new Date().toISOString();

function buildIdFile(): Plugin {
  return {
    name: "psynote-build-id",
    apply: "build",
    generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: "version.json",
        source: JSON.stringify({ build: BUILD_ID }),
      });
    },
  };
}

// Vite config (PsyNote frontend).
// Порт 5173 совпадает с топологией compose (docs/02 §7); хост-порт 5174.
// Прокси /api → gateway удобен в dev: фронт ходит на относительный /api,
// секреты и реальный адрес gateway остаются на стороне прокси (docs/07 §1).
// VITE_GATEWAY_URL — адрес gateway в docker-сети (gateway:8080) или localhost.
export default defineConfig({
  plugins: [react(), buildIdFile()],
  define: {
    __BUILD_ID__: JSON.stringify(BUILD_ID),
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_GATEWAY_URL ?? "http://gateway:8080",
        changeOrigin: true,
        timeout: 240_000,
        proxyTimeout: 240_000,
      },
    },
  },
});
