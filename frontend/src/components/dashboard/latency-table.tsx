import type { SummaryRow } from "@/api/bench"
import { VenueName } from "@/components/dashboard/venue-logo"
import { formatCount, formatLatency, formatPercent, formatUSD } from "@/lib/format"
import {
  cancelP50,
  cancelP95,
  confirmP50,
  confirmP95,
  primaryLabel,
  summarySpeedBumpMS,
} from "@/lib/latency-metric"
import { ArrowDown, ArrowUp, ChevronsUpDown } from "lucide-react"
import { useMemo, useState } from "react"

export function LatencyTable({
  isLoading = false,
  rows,
  subtractNetworkFloor,
}: {
  isLoading?: boolean
  rows: Array<SummaryRow>
  subtractNetworkFloor: boolean
}) {
  const [sort, setSort] = useState<SortState>({
    direction: "asc",
    key: "p95",
  })
  const sortedRows = useMemo(
    () => sortRows(rows, sort, subtractNetworkFloor),
    [rows, sort, subtractNetworkFloor]
  )

  return (
    <section className="overflow-hidden rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <h2 className="font-sans text-sm font-semibold">Venue Performance</h2>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[900px] border-collapse text-left text-[11px]">
          <thead className="bg-surface-2 text-muted-foreground">
            <tr>
              <HeaderCell>Venue</HeaderCell>
              <HeaderCell>Scenario</HeaderCell>
              <HeaderCell>Submission</HeaderCell>
              <HeaderCell>Order</HeaderCell>
              <HeaderCell>Metric</HeaderCell>
              <HeaderCell align="right">Measurements</HeaderCell>
              <HeaderCell align="right">OK</HeaderCell>
              <SortableHeaderCell
                active={sort.key === "p50"}
                align="right"
                direction={sort.direction}
                onClick={() => setSort((current) => nextSort(current, "p50"))}
              >
                p50
              </SortableHeaderCell>
              <SortableHeaderCell
                active={sort.key === "p95"}
                align="right"
                direction={sort.direction}
                onClick={() => setSort((current) => nextSort(current, "p95"))}
              >
                p95
              </SortableHeaderCell>
              <HeaderCell align="right">Cancel p50</HeaderCell>
              <HeaderCell align="right">Cancel p95</HeaderCell>
              <HeaderCell align="right">Cost/run</HeaderCell>
              <HeaderCell align="right">Speed bump</HeaderCell>
              <HeaderCell align="right">Errors</HeaderCell>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <TableLoadingRows />
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={14} className="px-3 py-8 text-muted-foreground">
                  No latency data is available for the selected filters.
                </td>
              </tr>
            ) : (
              sortedRows.map((row) => (
                <tr
                  key={`${row.venue}:${row.scenario}:${row.order_type}:${row.batch_size}:${row.batch_submission ?? ""}:${row.measurement_mode ?? "ack"}`}
                  className="border-t border-border/70"
                >
                  <BodyCell className="font-medium">
                    <VenueName venue={row.venue} />
                  </BodyCell>
                  <BodyCell>{row.scenario}</BodyCell>
                  <BodyCell>{submissionLabel(row)}</BodyCell>
                  <BodyCell>{row.order_type || "unknown"}</BodyCell>
                  <BodyCell>{primaryLabel(row.venue)}</BodyCell>
                  <BodyCell align="right">{formatCount(row.count)}</BodyCell>
                  <BodyCell align="right">{formatCount(row.ok)}</BodyCell>
                  <BodyCell align="right">{formatLatency(confirmP50(row, subtractNetworkFloor))}</BodyCell>
                  <BodyCell align="right">{formatLatency(confirmP95(row, subtractNetworkFloor))}</BodyCell>
                  <BodyCell align="right">{formatLatency(cancelP50(row, subtractNetworkFloor))}</BodyCell>
                  <BodyCell align="right">{formatLatency(cancelP95(row, subtractNetworkFloor))}</BodyCell>
                  <BodyCell align="right">{formatUSD(row.cost_mean_usd)}</BodyCell>
                  <BodyCell align="right">{formatLatency(summarySpeedBumpMS(row))}</BodyCell>
                  <BodyCell align="right">
                    {formatPercent(row.failed / Math.max(row.count, 1))}
                  </BodyCell>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  )
}

function TableLoadingRows() {
  return (
    <>
      {Array.from({ length: 5 }).map((_, index) => (
        <tr key={index} className="border-t border-border/70">
          {Array.from({ length: 14 }).map((__, cellIndex) => (
            <td key={cellIndex} className="px-3 py-3">
              <div
                className="h-3 animate-pulse rounded-sm bg-muted"
                style={{ width: `${cellIndex < 2 ? 72 : 44}%` }}
              />
            </td>
          ))}
        </tr>
      ))}
    </>
  )
}

type SortKey = "p50" | "p95"
type SortDirection = "asc" | "desc"

interface SortState {
  direction: SortDirection
  key: SortKey
}

function nextSort(current: SortState, key: SortKey): SortState {
  if (current.key !== key) {
    return { direction: "asc", key }
  }

  return {
    direction: current.direction === "asc" ? "desc" : "asc",
    key,
  }
}

function sortRows(
  rows: Array<SummaryRow>,
  sort: SortState,
  subtractNetworkFloor: boolean
) {
  const direction = sort.direction === "asc" ? 1 : -1
  return [...rows].sort((left, right) => {
    const latencyCompare = compareLatencyValues(
      sortValue(left, sort.key, subtractNetworkFloor),
      sortValue(right, sort.key, subtractNetworkFloor),
      direction
    )

    if (latencyCompare !== 0) {
      return latencyCompare
    }

    return compareRowIdentity(left, right)
  })
}

function sortValue(
  row: SummaryRow,
  key: SortKey,
  subtractNetworkFloor: boolean
) {
  return key === "p50"
    ? confirmP50(row, subtractNetworkFloor)
    : confirmP95(row, subtractNetworkFloor)
}

function compareLatencyValues(
  left: number | undefined,
  right: number | undefined,
  direction: 1 | -1
) {
  const leftValid = Number.isFinite(left)
  const rightValid = Number.isFinite(right)

  if (!leftValid && !rightValid) {
    return 0
  }
  if (!leftValid) {
    return 1
  }
  if (!rightValid) {
    return -1
  }

  return (Number(left) - Number(right)) * direction
}

function compareRowIdentity(left: SummaryRow, right: SummaryRow) {
  return `${left.venue}:${left.scenario}:${left.order_type}:${left.batch_size}:${left.batch_submission ?? ""}`.localeCompare(
    `${right.venue}:${right.scenario}:${right.order_type}:${right.batch_size}:${right.batch_submission ?? ""}`
  )
}

function submissionLabel(row: SummaryRow) {
  if (row.scenario !== "batch") {
    return "Single order"
  }
  const count = row.batch_size || 1
  if (row.batch_submission === "manual") {
    return `${count} manual fanout`
  }
  return `${count} native batch`
}

function HeaderCell({
  align = "left",
  children,
}: {
  align?: "left" | "right"
  children: React.ReactNode
}) {
  return (
    <th
      className="px-3 py-2 font-normal"
      style={{ textAlign: align }}
      scope="col"
    >
      {children}
    </th>
  )
}

function SortableHeaderCell({
  active,
  align = "left",
  children,
  direction,
  onClick,
}: {
  active: boolean
  align?: "left" | "right"
  children: React.ReactNode
  direction: SortDirection
  onClick: () => void
}) {
  const Icon = active ? (direction === "asc" ? ArrowUp : ArrowDown) : ChevronsUpDown

  return (
    <th
      aria-sort={active ? (direction === "asc" ? "ascending" : "descending") : "none"}
      className="px-3 py-2 font-normal"
      style={{ textAlign: align }}
      scope="col"
    >
      <button
        type="button"
        onClick={onClick}
        className={`inline-flex items-center gap-1 text-[11px] ${
          align === "right" ? "justify-end" : "justify-start"
        } w-full text-muted-foreground hover:text-foreground`}
      >
        <span>{children}</span>
        <Icon className="size-3" aria-hidden />
      </button>
    </th>
  )
}

function BodyCell({
  align = "left",
  children,
  className,
}: {
  align?: "left" | "right"
  children: React.ReactNode
  className?: string
}) {
  return (
    <td className={`tabular px-3 py-2 ${className ?? ""}`} style={{ textAlign: align }}>
      {children}
    </td>
  )
}
