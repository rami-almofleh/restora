import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// In development the UI runs on :5173 and forwards API calls to the Go API on :8080.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { "/api": "http://localhost:8080" },
  },
  build: {
    outDir: "dist",
    emptyOutDir: false,
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
