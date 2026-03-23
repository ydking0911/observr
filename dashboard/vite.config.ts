import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/events": "http://localhost:7676",
      "/query": "http://localhost:7676",
      "/ws": { target: "ws://localhost:7676", ws: true },
    },
  },
  build: {
    outDir: "../server/internal/dashboard/dist",
    emptyOutDir: true,
  },
});
