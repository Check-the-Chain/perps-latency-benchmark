import { WINDOW_OPTIONS, type WindowOption } from "@/api/bench"

export interface DashboardFilters {
  subtractNetworkFloor: boolean
  venues: "all" | Array<string>
  window: WindowOption
}

export function DashboardFilterBar({
  filters,
  onChange,
}: {
  filters: DashboardFilters
  onChange: (filters: DashboardFilters) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <FilterSelect
        label="Window"
        value={filters.window}
        options={WINDOW_OPTIONS.map((value) => ({ label: value, value }))}
        onChange={(window) =>
          onChange({ ...filters, window: window as WindowOption })
        }
      />
    </div>
  )
}

function FilterSelect({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: Array<{ label: string; value: string }>
  value: string
  onChange: (value: string) => void
}) {
  return (
    <label className="flex items-center gap-2 rounded-sm border border-border bg-surface-1 px-2 py-1.5 text-[11px] text-muted-foreground">
      <span>{label}</span>
      <select
        value={value}
        onChange={(event) => onChange(event.currentTarget.value)}
        className="bg-transparent text-foreground outline-none"
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  )
}
