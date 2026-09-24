const INFRASTRUCTURE_ITEMS = [
  { label: "Cloud", value: "AWS EC2" },
  { label: "Location", value: "Tokyo (ap-northeast-1)" },
  { label: "Availability zone", value: "ap-northeast-1a" },
  { label: "Instance", value: "t2.medium" },
]

export function InfrastructurePanel() {
  return (
    <section className="rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <h2 className="font-sans text-sm font-semibold">Benchmark Box</h2>
      </div>
      <div className="grid gap-px bg-border/60 sm:grid-cols-2 lg:grid-cols-4">
        {INFRASTRUCTURE_ITEMS.map((item) => (
          <div key={item.label} className="bg-surface-1 p-3">
            <div className="text-[10px] uppercase text-muted-foreground">
              {item.label}
            </div>
            <div className="tabular mt-2 text-[12px] font-medium text-foreground">
              {item.value}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}
