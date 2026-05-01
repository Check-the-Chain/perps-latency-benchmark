import type { SummaryRow } from "@/api/bench"
import { formatCount, formatLatency, formatPercent } from "@/lib/format"

export function LatencyTable({ rows }: { rows: Array<SummaryRow> }) {
  return (
    <section className="overflow-hidden rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <h2 className="font-sans text-sm font-semibold">Venue Performance</h2>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] border-collapse text-left text-[11px]">
          <thead className="bg-surface-2 text-muted-foreground">
            <tr>
              <HeaderCell>Venue</HeaderCell>
              <HeaderCell>Transport</HeaderCell>
              <HeaderCell>Scenario</HeaderCell>
              <HeaderCell>Order</HeaderCell>
              <HeaderCell align="right">Measurements</HeaderCell>
              <HeaderCell align="right">OK</HeaderCell>
              <HeaderCell align="right">p50</HeaderCell>
              <HeaderCell align="right">p95</HeaderCell>
              <HeaderCell align="right">p99</HeaderCell>
              <HeaderCell align="right">Errors</HeaderCell>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={10} className="px-3 py-8 text-muted-foreground">
                  No latency data is available for the selected filters.
                </td>
              </tr>
            ) : (
              rows.map((row) => (
                <tr
                  key={`${row.venue}:${row.transport}:${row.scenario}:${row.order_type}`}
                  className="border-t border-border/70"
                >
                  <BodyCell className="font-medium">{row.venue}</BodyCell>
                  <BodyCell>{row.transport}</BodyCell>
                  <BodyCell>{row.scenario}</BodyCell>
                  <BodyCell>{row.order_type || "unknown"}</BodyCell>
                  <BodyCell align="right">{formatCount(row.count)}</BodyCell>
                  <BodyCell align="right">{formatCount(row.ok)}</BodyCell>
                  <BodyCell align="right">{formatLatency(row.p50_ms)}</BodyCell>
                  <BodyCell align="right">{formatLatency(row.p95_ms)}</BodyCell>
                  <BodyCell align="right">{formatLatency(row.p99_ms)}</BodyCell>
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
