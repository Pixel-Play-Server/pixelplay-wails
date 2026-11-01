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
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return undefined;
          if (/[\\/]ogl[\\/]/.test(id)) return "vendor-ogl";
          if (/[\\/](three|@react-three|skinview3d|@flasham)[\\/]/.test(id)) return "vendor-3d";
          if (/[\\/](react-markdown|remark-|rehype-|micromark|mdast-|hast-|unified|vfile)/.test(id)) return "vendor-markdown";
          if (/[\\/](react|react-dom|react-router|react-router-dom|scheduler)[\\/]/.test(id)) return "vendor-react";
          if (/[\\/](i18next|react-i18next|i18next-[^/\\]+)[\\/]/.test(id)) return "vendor-i18n";
          if (/[\\/](gsap|date-fns|flagsmith|@iconify)[\\/]/.test(id)) return "vendor-misc";
          return "vendor";
        },
      },
    },
  },
});
