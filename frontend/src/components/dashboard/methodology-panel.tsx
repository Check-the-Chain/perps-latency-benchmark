const METHODOLOGIES = [
  {
    venue: "Hyperliquid",
    text: "Post-only BTC order. Confirm latency is measured from completed order-submit write to the matching private account-feed order update. The synchronous exchange response is shown on hover.",
  },
  {
    venue: "Lighter",
    text: "Post-only BTC order using the maker-only API key. Confirm latency is measured from completed sendTx write to the matching private account order/trade WebSocket event for the client order index. The sendTx queue ack is shown on hover.",
  },
  {
    venue: "Extended",
    text: "Post-only BTC order. Confirm latency is measured from completed order-submit write to the matching private account order/trade WebSocket event for the external order ID. Extended taker latency is adjusted by its 150 ms order-entry speed bump. Batch view labels documented native batch separately from manual fanout.",
  },
  {
    venue: "Nado",
    text: "Post-only BTC-PERP order. Single-order submit uses Gateway WebSocket and private subscription confirmation. Batch view uses manual fanout over concurrent place_order executes because no native multi-place endpoint is documented.",
  },
  {
    venue: "Aster",
    text: "Post-only BTCUSDT order using GTX time-in-force. Confirm latency is measured from completed order-submit write to the matching user data stream ORDER_TRADE_UPDATE event for the client order ID. The HTTP submit response is shown on hover.",
  },
]

export function MethodologyPanel() {
  return (
    <section className="rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <h2 className="font-sans text-sm font-semibold">Methodology</h2>
      </div>
      <div className="grid gap-px bg-border/60 md:grid-cols-5">
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
