import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
export default defineConfig({
  plugins: [
    react(),
    {
      name: "development-csp",
      apply: "serve",
      transformIndexHtml(html) {
        return html.replace(
          "script-src 'self';",
          "script-src 'self' 'unsafe-inline';",
        );
      },
    },
  ],
  test: { environment: "jsdom", clearMocks: true },
});
