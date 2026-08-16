import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import tailwindcss from "@tailwindcss/vite";

const apiTarget = process.env.CODEFORGE_URL || "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        // Vite 8 bundles with rolldown, which drops the object form of
        // `manualChunks` in favour of `advancedChunks.groups`.
        advancedChunks: {
          groups: [
            {
              name: "vendor",
              test: /[\\/]node_modules[\\/](react|react-dom|react-router|scheduler)[\\/]/,
            },
            {
              name: "query",
              test: /[\\/]node_modules[\\/]@tanstack[\\/]react-query[\\/]/,
            },
          ],
        },
      },
    },
  },
});
