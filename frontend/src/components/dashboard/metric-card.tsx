import { cn } from "@/lib/utils"

export function MetricCard({
  label,
  value,
  detail,
  tone = "neutral",
}: {
  label: string
  value: string
  detail: string
  tone?: "neutral" | "good" | "warning" | "bad"
}) {
  return (
    <div className="rounded-sm border border-border/80 bg-surface-1 p-3">
      <div className="text-[10px] uppercase text-muted-foreground">
        {label}
      </div>
      <div
        className={cn(
          "tabular mt-2 font-sans text-2xl font-semibold tracking-normal",
          tone === "good" && "text-profit",
          tone === "warning" && "text-warning",
          tone === "bad" && "text-loss"
        )}
      >
        {value}
      </div>
      <div className="mt-1 truncate text-[11px] text-muted-foreground">
        {detail}
      </div>
    </div>
  )
}
