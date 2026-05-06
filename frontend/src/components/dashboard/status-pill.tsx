import { cn } from "@/lib/utils"

type StatusTone = "good" | "neutral" | "warning" | "bad"

const toneClassName: Record<StatusTone, string> = {
  good: "border-profit/25 bg-profit/10 text-profit",
  neutral: "border-border bg-surface-1 text-muted-foreground",
  warning: "border-warning/25 bg-warning/10 text-warning",
  bad: "border-loss/25 bg-loss/10 text-loss",
}

export function StatusPill({
  children,
  tone = "neutral",
}: {
  children: React.ReactNode
  tone?: StatusTone
}) {
  return (
    <span
      className={cn(
        "inline-flex h-7 items-center rounded-sm border px-2 text-[11px]",
        toneClassName[tone]
      )}
    >
      {children}
    </span>
  )
}
