import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "path";

const shims = resolve(__dirname, "shims");

export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    strictPort: true,
  },
  resolve: {
    alias: {
      "@tauri-apps/api/core": resolve(shims, "tauri-core.ts"),
      "@tauri-apps/api/event": resolve(shims, "tauri-event.ts"),
      "@tauri-apps/api/window": resolve(shims, "tauri-window.ts"),
      "@tauri-apps/api/webview": resolve(shims, "tauri-webview.ts"),
      "@tauri-apps/api/webviewWindow": resolve(shims, "tauri-window.ts"),
      "@tauri-apps/api/app": resolve(shims, "tauri-core.ts"),
      "@tauri-apps/plugin-opener": resolve(shims, "tauri-opener.ts"),
      "@tauri-apps/plugin-dialog": resolve(shims, "tauri-dialog.ts"),
      "@tauri-apps/plugin-process": resolve(shims, "tauri-process.ts"),
      "@tauri-apps/plugin-clipboard-manager": resolve(shims, "tauri-clipboard.ts"),
      "@tauri-apps/plugin-fs": resolve(shims, "tauri-fs.ts"),
      "@tauri-apps/plugin-shell": resolve(shims, "tauri-shell.ts"),
      "@tauri-apps/plugin-updater": resolve(shims, "tauri-updater.ts"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        main: resolve(__dirname, "index.html"),
      },
    },
  },
});
