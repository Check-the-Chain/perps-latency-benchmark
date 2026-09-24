import { tanstackStart } from "@tanstack/react-start/plugin/vite"
import viteReact from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { cloudflare } from "@cloudflare/vite-plugin"
import { defineConfig } from "vite"
import viteTsConfigPaths from "vite-tsconfig-paths"

export default defineConfig({
  plugins: [
    cloudflare({
      viteEnvironment: { name: "ssr" },
    }),
    viteTsConfigPaths(),
    tailwindcss(),
    tanstackStart(),
    viteReact(),
  ],
})
