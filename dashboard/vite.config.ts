import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { readFileSync } from "fs";

const { version } = JSON.parse(readFileSync("package.json", "utf-8")) as { version: string };

export default defineConfig({
  plugins: [react()],
  define: {
    __APP_VERSION__: JSON.stringify(version),
  },
  server: {
    proxy: {
      "/events": "http://localhost:7676",
      "/query": "http://localhost:7676",
      "/patterns": "http://localhost:7676",
      "/ws": { target: "ws://localhost:7676", ws: true },
    },
  },
  build: {
    outDir: "../server/internal/dashboard/dist",
    emptyOutDir: true,
  },
});
