import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Builds the SPA only. The Go server (apps/server) embeds dist/client via
// go:embed; in development it runs on :3000 and Vite proxies the server-owned
// paths to it so the client's same-origin assumption (and cookies) hold.
const serverPaths = ["/api", "/l", "/healthz", "/readyz", "/openapi.json", "/scalar", "/images", "/robots.txt"];

export default defineConfig({
  define: {
    // Reported by the settings page; the Go server reports its own version on /healthz.
    __APP_VERSION__: JSON.stringify(process.env.APP_VERSION ?? "dev"),
  },
  build: {
    outDir: "../../dist/client",
    emptyOutDir: true,
  },
  server: {
    proxy: Object.fromEntries(serverPaths.map((p) => [p, "http://localhost:3000"])),
  },
  plugins: [react()],
});
