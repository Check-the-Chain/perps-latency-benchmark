import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"

export type Theme = "dark" | "light" | "system"

type ThemeProviderState = {
  resolvedTheme: "dark" | "light"
  setTheme: (theme: Theme) => void
  theme: Theme
}

const STORAGE_KEY = "perps-bench-theme"
const DEFAULT_THEME: Theme = "system"
const ThemeProviderContext = createContext<ThemeProviderState | null>(null)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(DEFAULT_THEME)
  const [resolvedTheme, setResolvedTheme] = useState<"dark" | "light">("light")

  useEffect(() => {
    setThemeState(storedTheme() ?? DEFAULT_THEME)
  }, [])

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)")

    const apply = () => {
      const nextResolved =
        theme === "system" ? (media.matches ? "dark" : "light") : theme
      document.documentElement.classList.toggle("dark", nextResolved === "dark")
      document.documentElement.style.colorScheme = nextResolved
      setResolvedTheme(nextResolved)
    }

    apply()
    media.addEventListener("change", apply)
    return () => media.removeEventListener("change", apply)
  }, [theme])

  const value = useMemo<ThemeProviderState>(
    () => ({
      resolvedTheme,
      setTheme: (nextTheme) => {
        localStorage.setItem(STORAGE_KEY, nextTheme)
        setThemeState(nextTheme)
      },
      theme,
    }),
    [resolvedTheme, theme]
  )

  return (
    <ThemeProviderContext.Provider value={value}>
      {children}
    </ThemeProviderContext.Provider>
  )
}

export function ThemeScript() {
  return (
    <script
      suppressHydrationWarning
      dangerouslySetInnerHTML={{
        __html: `try{const stored=localStorage.getItem(${JSON.stringify(STORAGE_KEY)});const theme=stored==="dark"||stored==="light"||stored==="system"?stored:${JSON.stringify(DEFAULT_THEME)};const resolved=theme==="system"?(window.matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light"):theme;document.documentElement.classList.toggle("dark",resolved==="dark");document.documentElement.style.colorScheme=resolved}catch{document.documentElement.classList.remove("dark");document.documentElement.style.colorScheme="light"}`,
      }}
    />
  )
}

export function useTheme() {
  const context = useContext(ThemeProviderContext)
  if (!context) {
    throw new Error("useTheme must be used within ThemeProvider")
  }
  return context
}

function storedTheme(): Theme | null {
  const value = localStorage.getItem(STORAGE_KEY)
  return value === "dark" || value === "light" || value === "system"
    ? value
    : null
}
