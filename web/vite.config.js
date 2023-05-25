/* eslint-disable import/no-extraneous-dependencies */
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(() => ({
  build: {
    outDir: "build",
    assetsDir: "static/media",
  },
  server: {
    port: 3000,
  },
  plugins: [react()],
}));
