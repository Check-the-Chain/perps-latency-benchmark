"use client"

import { Monitor, Moon, Sun } from "lucide-react"

import { useTheme, type Theme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

const OPTIONS: Array<{ icon: typeof Sun; label: string; value: Theme }> = [
  { icon: Sun, label: "Light", value: "light" },
  { icon: Moon, label: "Dark", value: "dark" },
  { icon: Monitor, label: "System", value: "system" },
]

export function ThemeToggle() {
  const { setTheme, theme } = useTheme()

  return (
    <div
      aria-label="Theme"
      className="inline-flex h-8 overflow-hidden rounded-sm border border-border bg-surface-1 text-[11px]"
      role="group"
    >
      {OPTIONS.map((option) => {
        const Icon = option.icon
        const active = option.value === theme

        return (
          <button
            key={option.value}
            type="button"
            aria-pressed={active}
            className={cn(
              "inline-flex items-center gap-1.5 px-2 text-muted-foreground hover:bg-surface-2 hover:text-foreground",
              active && "bg-foreground text-background hover:bg-foreground hover:text-background"
            )}
            onClick={() => setTheme(option.value)}
            title={`${option.label} theme`}
          >
            <Icon className="size-3.5" aria-hidden />
            <span className="hidden sm:inline">{option.label}</span>
          </button>
        )
      })}
    </div>
  )
}
