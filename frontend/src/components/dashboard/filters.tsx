import { WINDOW_OPTIONS, type WindowOption } from "@/api/bench"

export interface DashboardFilters {
  scenario: string
  transport: string
  venue: string
  window: WindowOption
}

export function DashboardFilterBar({
  filters,
  scenarios,
  transports,
  venues,
  onChange,
}: {
  filters: DashboardFilters
  scenarios: Array<string>
  transports: Array<string>
  venues: Array<string>
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
      <FilterSelect
        label="Venue"
        value={filters.venue}
        options={withAll(venues)}
        onChange={(venue) => onChange({ ...filters, venue })}
      />
      <FilterSelect
        label="Transport"
        value={filters.transport}
        options={withAll(transports)}
        onChange={(transport) => onChange({ ...filters, transport })}
      />
      <FilterSelect
        label="Scenario"
        value={filters.scenario}
        options={withAll(scenarios)}
        onChange={(scenario) => onChange({ ...filters, scenario })}
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

function withAll(values: Array<string>) {
  return [
    { label: "all", value: "all" },
    ...values.map((value) => ({ label: value, value })),
  ]
}
