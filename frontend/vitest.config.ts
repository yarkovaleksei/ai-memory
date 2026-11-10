import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// Отдельно от vite.config.ts: тестам не нужен плагин Wails и dev-сервер.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
