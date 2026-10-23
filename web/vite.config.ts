import path from "node:path";
import { readFileSync } from "node:fs";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), {
    name: 'mediamtx-license',
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'third-party/mediamtx-LICENSE.txt', source: readFileSync(new URL('./src/lib/vendor/mediamtx/LICENSE', import.meta.url), 'utf8') });
    },
  }],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});

