const METHODOLOGIES = [
  {
    venue: "Hyperliquid",
    text: "Post-only BTC order. Primary latency is measured from completed order-submit write to the matching private orderUpdates WebSocket event for the client order ID. Ack latency is shown separately as submit-response timing.",
  },
  {
    venue: "Lighter",
    text: "Post-only BTC order using the maker-only API key. Primary latency is measured from completed sendTx write to the matching private account order/trade WebSocket event for the client order index. Ack latency is shown separately as sendTx queue response timing.",
  },
]

export function MethodologyPanel() {
  return (
    <section className="rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <h2 className="font-sans text-sm font-semibold">Methodology</h2>
      </div>
      <div className="grid gap-px bg-border/60 md:grid-cols-2">
        {METHODOLOGIES.map((item) => (
          <div key={item.venue} className="bg-surface-1 p-3">
            <div className="text-[10px] uppercase text-muted-foreground">
              {item.venue}
            </div>
            <p className="mt-2 text-[11px] leading-5 text-foreground">
              {item.text}
            </p>
          </div>
        ))}
      </div>
    </section>
  )
}
