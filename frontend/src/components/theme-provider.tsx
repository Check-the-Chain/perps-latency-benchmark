import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"
import { ScriptOnce } from "@tanstack/react-router"

export type Theme = "dark" | "light" | "system"

type ThemeProviderProps = {
  children: ReactNode
  defaultTheme?: Theme
  storageKey?: string
}

type ThemeProviderState = {
  resolvedTheme: "dark" | "light"
  setTheme: (theme: Theme) => void
  theme: Theme
}

const ThemeProviderContext = createContext<ThemeProviderState>({
  resolvedTheme: "light",
  theme: "system",
  setTheme: () => {},
})

function resolveTheme(theme: Theme) {
  return theme === "system"
    ? window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light"
    : theme
}

function applyTheme(theme: Theme) {
  const root = document.documentElement
  const resolved = resolveTheme(theme)

  root.classList.remove("light", "dark")
  root.classList.add(resolved)
  root.style.colorScheme = resolved

  return resolved
}

export function ThemeProvider({
  children,
  defaultTheme = "system",
  storageKey = "theme",
}: ThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(defaultTheme)
  const [resolvedTheme, setResolvedTheme] = useState<"dark" | "light">("light")
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setThemeState(storedTheme(storageKey) ?? defaultTheme)
    setMounted(true)
  }, [defaultTheme, storageKey])

  useEffect(() => {
    if (!mounted) return

    setResolvedTheme(applyTheme(theme))
  }, [mounted, theme])

  useEffect(() => {
    if (!mounted || theme !== "system") return

    const media = window.matchMedia("(prefers-color-scheme: dark)")
    const onChange = () => setResolvedTheme(applyTheme("system"))

    media.addEventListener("change", onChange)
    return () => media.removeEventListener("change", onChange)
  }, [mounted, theme])

  const value = useMemo<ThemeProviderState>(
    () => ({
      resolvedTheme,
      setTheme: (nextTheme) => {
        localStorage.setItem(storageKey, nextTheme)
        setThemeState(nextTheme)
      },
      theme,
    }),
    [resolvedTheme, storageKey, theme]
  )

  return (
    <ThemeProviderContext.Provider value={value}>
      <ScriptOnce>{getThemeScript(storageKey, defaultTheme)}</ScriptOnce>
      {children}
    </ThemeProviderContext.Provider>
  )
}

export function useTheme() {
  const context = useContext(ThemeProviderContext)
  return context
}

function storedTheme(storageKey: string): Theme | null {
  const value = localStorage.getItem(storageKey)
  return value === "dark" || value === "light" || value === "system"
    ? value
    : null
}

function getThemeScript(storageKey: string, defaultTheme: Theme) {
  const key = JSON.stringify(storageKey)
  const fallback = JSON.stringify(defaultTheme)
  return `(function(){try{var t=localStorage.getItem(${key});if(t!=='light'&&t!=='dark'&&t!=='system'){t=${fallback}}var d=matchMedia('(prefers-color-scheme: dark)').matches;var r=t==='system'?(d?'dark':'light'):t;var e=document.documentElement;e.classList.add(r);e.style.colorScheme=r}catch(e){}})();`
}
