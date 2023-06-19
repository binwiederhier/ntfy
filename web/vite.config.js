/* eslint-disable import/no-extraneous-dependencies */
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";

export default defineConfig(() => ({
  build: {
    outDir: "build",
    assetsDir: "static/media",
    sourcemap: true,
  },
  server: {
    port: 3000,
  },
  plugins: [
    react(),
    VitePWA({
      registerType: "autoUpdate",
      injectRegister: "inline",
      strategies: "injectManifest",
      devOptions: {
        enabled: true,
        /* when using generateSW the PWA plugin will switch to classic */
        type: "module",
        navigateFallback: "index.html",
      },
      injectManifest: {
        globPatterns: ["**/*.{js,css,html,mp3,ico,png,svg,json}"],
        globIgnores: ["config.js"],
        manifestTransforms: [
          (entries) => ({
            manifest: entries.map((entry) =>
              // this matches the build step in the Makefile.
              // since ntfy needs the ability to serve another page on /index.html,
              // it's renamed and served from server.go as app.html as well.
              entry.url === "index.html"
                ? {
                    ...entry,
                    url: "app.html",
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
