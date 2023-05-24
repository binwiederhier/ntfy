/* eslint-disable import/no-extraneous-dependencies */
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";

// please look at develop.md for how to run your browser
// in a mode allowing insecure service worker testing
// this turns on:
// - the service worker in dev mode
// - turns off automatically opening the browser
const enableLocalPWATesting = process.env.ENABLE_DEV_PWA;

export default defineConfig(() => ({
  build: {
    outDir: "build",
    assetsDir: "static/media",
    sourcemap: true,
  },
  server: {
    port: 3000,
    open: !enableLocalPWATesting,
  },
  plugins: [
    react(),
    VitePWA({
      registerType: "autoUpdate",
      injectRegister: "inline",
      strategies: "injectManifest",
      devOptions: {
        enabled: enableLocalPWATesting,
        /* when using generateSW the PWA plugin will switch to classic */
        type: "module",
        navigateFallback: "index.html",
      },
      injectManifest: {
        globPatterns: ["**/*.{js,css,html,mp3,png,svg,json}"],
        globIgnores: ["config.js"],
        manifestTransforms: [
          (entries) => ({
            manifest: entries.map((entry) =>
              entry.url === "index.html"
                ? {
                    ...entry,
                    url: "/",
                  }
                : entry
            ),
          }),
        ],
      },
      manifest: {
        name: "ntfy web",
        short_name: "ntfy",
        description:
          "ntfy lets you send push notifications via scripts from any computer or phone. Made with ❤ by Philipp C. Heckel, Apache License 2.0, source at https://heckel.io/ntfy.",
        theme_color: "#317f6f",
        start_url: "/",
        icons: [
          {
            src: "/static/images/pwa-192x192.png",
            sizes: "192x192",
            type: "image/png",
          },
          {
            src: "/static/images/pwa-512x512.png",
            sizes: "512x512",
            type: "image/png",
          },
        ],
      },
    }),
  ],
}));
