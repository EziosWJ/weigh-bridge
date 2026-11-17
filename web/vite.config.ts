import path from "node:path";
import { readFileSync } from "node:fs";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const proxyTarget = env.VITE_API_PROXY_TARGET || "http://127.0.0.1:8900";
  return {
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
    server: {
      host: "0.0.0.0",
      proxy: {
        "/api": { target: proxyTarget, changeOrigin: true },
      },
    },
    preview: { host: "0.0.0.0" },
  };
});

