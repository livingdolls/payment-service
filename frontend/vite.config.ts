import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig, type ProxyOptions, type ViteDevServer } from "vite";
import { loadLiteralEnv } from "./config/env.ts";

export default defineConfig(({ mode }) => {
  const frontendRoot = fileURLToPath(new URL(".", import.meta.url));
  const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
  const env = {
    ...loadLiteralEnv(repositoryRoot, mode),
    ...loadLiteralEnv(frontendRoot, mode),
    ...process.env,
  };
  const target =
    env.PAYMENT_SERVICE_URL || `http://127.0.0.1:${env.HTTP_PORT || "8080"}`;

  const proxy = (
    prefix: string,
    destination: string,
    token?: string,
  ): ProxyOptions => ({
    target,
    changeOrigin: true,
    rewrite: (path) => path.replace(prefix, destination),
    configure(server) {
      server.on("proxyReq", (request) => {
        request.removeHeader("authorization");
        if (token) request.setHeader("Authorization", `Bearer ${token}`);
      });
      server.on("error", (_error, _request, response) => {
        if ("writeHead" in response && !response.headersSent) {
          response.writeHead(502, { "Content-Type": "application/json" });
          response.end(
            JSON.stringify({
              error: {
                code: "API_UNAVAILABLE",
                message:
                  "API belum dapat dihubungi. Jalankan payment-service dan periksa konfigurasi koneksi.",
              },
            }),
          );
        }
      });
    },
  });

  const proxies = {
    "/api/payments": proxy(
      "/api/payments",
      "/internal/payments",
      env.PAYMENT_INTERNAL_TOKEN,
    ),
    "/api/payment-attempts": proxy(
      "/api/payment-attempts",
      "/internal/payment-attempts",
      env.PAYMENT_INTERNAL_TOKEN,
    ),
    "/api/admin": proxy("/api/admin", "/admin", env.ADMIN_API_TOKEN),
    "/api/health": proxy("/api/health", "/health"),
  };

  const configureConsole = (server: Pick<ViteDevServer, "middlewares">) => {
    server.middlewares.use("/api", (request, response, next) => {
      const origin = request.headers.origin;
      if (origin) {
        try {
          if (new URL(origin).host !== request.headers.host) {
            response.writeHead(403, { "Content-Type": "application/json" });
            response.end(
              JSON.stringify({
                error: {
                  code: "ORIGIN_REJECTED",
                  message: "Origin permintaan tidak diizinkan.",
                },
              }),
            );
            return;
          }
        } catch {
          response.writeHead(403, { "Content-Type": "application/json" });
          response.end(
            JSON.stringify({
              error: {
                code: "ORIGIN_REJECTED",
                message: "Origin permintaan tidak valid.",
              },
            }),
          );
          return;
        }
      }
      next();
    });
    server.middlewares.use("/api/console-config", (_request, response) => {
      response.setHeader("Content-Type", "application/json");
      response.setHeader("Cache-Control", "no-store");
      response.end(
        JSON.stringify({
          apiUrl: target,
          environment: env.APP_ENV || "development",
          internalConfigured: (env.PAYMENT_INTERNAL_TOKEN?.length || 0) >= 32,
          adminConfigured: (env.ADMIN_API_TOKEN?.length || 0) >= 32,
        }),
      );
    });
  };

  return {
    plugins: [
      react(),
      {
        name: "payment-console-config",
        configureServer: configureConsole,
        configurePreviewServer: configureConsole,
      },
    ],
    server: { port: 5173, strictPort: true, proxy: proxies },
    preview: { port: 4173, strictPort: true, proxy: proxies },
    build: { sourcemap: false },
  };
});
