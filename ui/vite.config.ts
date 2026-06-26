import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// The panel is served by the Go backend under /panel/* and embedded into the
// binary. Build output goes straight into the Go embed directory.
const backend = "http://localhost:8080";

export default defineConfig({
  base: "/panel/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "~": new URL("./app", import.meta.url).pathname,
    },
  },
  build: {
    outDir: "../internal/server/webui/dist",
    // Keep the tracked placeholder index.html / .gitignore in place; hashed
    // asset filenames make stale buildup a non-issue.
    emptyOutDir: false,
  },
  server: {
    // Dev: the SPA runs on Vite while the Go backend serves the RPC API.
    proxy: {
      "/skylex.v1.": { target: backend, changeOrigin: true },
      "/version": backend,
      "/install-agent.sh": backend,
      "/skylex-agent": backend,
    },
  },
});
