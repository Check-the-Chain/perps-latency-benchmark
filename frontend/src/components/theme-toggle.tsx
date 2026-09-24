"use client"

import { Moon, Sun } from "lucide-react"

import { useTheme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()
  const isDark = resolvedTheme === "dark"

  return (
    <button
      type="button"
      aria-label={isDark ? "Switch to light theme" : "Switch to dark theme"}
      aria-pressed={isDark}
      className="relative inline-flex size-8 items-center justify-center overflow-hidden rounded-sm border border-border bg-surface-1 text-muted-foreground transition-colors hover:bg-surface-2 hover:text-foreground"
      onClick={() => setTheme(isDark ? "light" : "dark")}
      title={isDark ? "Switch to light theme" : "Switch to dark theme"}
    >
      <Sun
        className={cn(
          "absolute size-4 transition-all duration-300 ease-out",
          isDark ? "-rotate-90 scale-0 opacity-0" : "rotate-0 scale-100 opacity-100"
        )}
        aria-hidden
      />
      <Moon
        className={cn(
          "absolute size-4 transition-all duration-300 ease-out",
          isDark ? "rotate-0 scale-100 opacity-100" : "rotate-90 scale-0 opacity-0"
        )}
        aria-hidden
      />
    </button>
  )
}
