import express from "express";
import { createProxyMiddleware } from "http-proxy-middleware";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const app = express();
const PORT = 3000;
const isDev = process.env.NODE_ENV !== "production";
const codeforgeUrl = process.env.CODEFORGE_URL || "http://localhost:8080";

// Backend serves /health and /metrics at root, but the UI calls /api/v1/health etc.
// Express strips the mount path before passing to proxy, so req.url is "/"
app.use(
  "/api/v1/health",
  createProxyMiddleware({
    target: codeforgeUrl,
    changeOrigin: true,
    pathRewrite: { "^/": "/health" },
  }),
);
app.use(
  "/api/v1/metrics",
  createProxyMiddleware({
    target: codeforgeUrl,
    changeOrigin: true,
    pathRewrite: { "^/": "/metrics" },
  }),
);

// API proxy — Express strips the mount path "/api", so we prepend it back
app.use(
  "/api",
  createProxyMiddleware({
    target: codeforgeUrl,
    changeOrigin: true,
    pathRewrite: (p) => `/api${p}`,
  }),
);

if (isDev) {
  const { createServer } = await import("vite");
  const vite = await createServer({
    server: { middlewareMode: true },
    appType: "spa",
  });
  app.use(vite.middlewares);
} else {
  const { default: compression } = await import("compression");
  app.use(compression());
  app.use(express.static(path.resolve(__dirname, "dist")));
  app.get("*", (_req, res) => {
    res.sendFile(path.resolve(__dirname, "dist", "index.html"));
  });
}

app.listen(PORT, "0.0.0.0", () => {
  console.log(
    `Server running in ${isDev ? "development" : "production"} mode on http://localhost:${PORT}`,
  );
});
