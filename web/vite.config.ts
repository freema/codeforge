import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import tailwindcss from "@tailwindcss/vite";

const apiTarget = process.env.CODEFORGE_URL || "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    proxy: {
      "/api/v1/health": {
        target: apiTarget,
        changeOrigin: true,
        rewrite: () => "/health",
      },
      "/api/v1/metrics": {
        target: apiTarget,
        changeOrigin: true,
        rewrite: () => "/metrics",
      },
      "/api": {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ["react", "react-dom", "react-router"],
          query: ["@tanstack/react-query"],
          charts: ["recharts"],
        },
      },
    },
  },
});
