"use client"

import { Download } from "lucide-react"
import { useEffect, useState } from "react"

import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"

type CSVValue = string | number | boolean | Date | null | undefined

export interface CSVColumn<T> {
  header: string
  value: (row: T) => CSVValue
}

export interface CSVWindowOption {
  label: string
  value: string
}

export function CSVExportButton<T>({
  columns,
  disabled = false,
  filename,
  filenameForWindow,
  label = "CSV",
  loadRows,
  defaultWindow,
  rows,
  windowOptions,
}: {
  columns: Array<CSVColumn<T>>
  disabled?: boolean
  filename: string
  filenameForWindow?: (window: string) => string
  label?: string
  loadRows?: (window: string) => Promise<Array<T>>
  defaultWindow?: string
  rows: Array<T>
  windowOptions?: ReadonlyArray<CSVWindowOption>
}) {
  const windows = windowOptions ?? []
  const initialWindow = defaultWindow ?? windows[0]?.value ?? ""
  const [open, setOpen] = useState(false)
  const [selectedWindow, setSelectedWindow] = useState(initialWindow)
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)

  useEffect(() => {
    setSelectedWindow(initialWindow)
  }, [initialWindow])

  async function handleExport() {
    setPending(true)
    setError("")
    try {
      const exportRows = loadRows ? await loadRows(selectedWindow) : rows
      if (exportRows.length === 0) {
        setError("No data for that window")
        return
      }
      downloadCSV(
        filenameForWindow?.(selectedWindow) ?? filename,
        columns,
        exportRows
      )
      setOpen(false)
    } catch {
      setError("Export failed")
    } finally {
      setPending(false)
    }
  }

  return (
    <CSVDialogShell
      disabled={disabled || (!loadRows && rows.length === 0)}
      error={error}
      label={label}
      onExport={handleExport}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) {
          setError("")
        }
      }}
      open={open}
      pending={pending}
      selectedWindow={selectedWindow}
      setSelectedWindow={setSelectedWindow}
      title={rows.length === 0 && !loadRows ? "No data to export" : "Export CSV"}
      windowOptions={windows}
    />
  )
}

export function CSVURLExportButton({
  defaultWindow,
  disabled = false,
  filenameForWindow,
  hrefForWindow,
  label = "CSV",
  windowOptions,
}: {
  defaultWindow?: string
  disabled?: boolean
  filenameForWindow: (window: string) => string
  hrefForWindow: (window: string) => string
  label?: string
  windowOptions: ReadonlyArray<CSVWindowOption>
}) {
  const initialWindow = defaultWindow ?? windowOptions[0]?.value ?? ""
  const [open, setOpen] = useState(false)
  const [selectedWindow, setSelectedWindow] = useState(initialWindow)
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)

  useEffect(() => {
    setSelectedWindow(initialWindow)
  }, [initialWindow])

  async function handleExport() {
    setPending(true)
    setError("")
    try {
      const response = await fetch(hrefForWindow(selectedWindow))
      if (!response.ok) {
        throw new Error(`Export failed: ${response.status}`)
      }
      downloadBlob(
        filenameFromDisposition(response.headers.get("Content-Disposition")) ??
          filenameForWindow(selectedWindow),
        await response.blob()
      )
      setOpen(false)
    } catch {
      setError("Export failed")
    } finally {
      setPending(false)
    }
  }

  return (
    <CSVDialogShell
      disabled={disabled}
      error={error}
      label={label}
      onExport={handleExport}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) {
          setError("")
        }
      }}
      open={open}
      pending={pending}
      selectedWindow={selectedWindow}
      setSelectedWindow={setSelectedWindow}
      title="Export CSV"
      windowOptions={windowOptions}
    />
  )
}

function CSVDialogShell({
  disabled,
  error,
  label,
  onExport,
  onOpenChange,
  open,
  pending,
  selectedWindow,
  setSelectedWindow,
  title,
  windowOptions,
}: {
  disabled: boolean
  error: string
  label: string
  onExport: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
  pending: boolean
  selectedWindow: string
  setSelectedWindow: (window: string) => void
  title: string
  windowOptions: ReadonlyArray<CSVWindowOption>
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger asChild>
        <button
          type="button"
          className="inline-flex h-8 items-center gap-1 rounded-sm border border-border bg-surface-1 px-2 text-[11px] text-foreground hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-50"
          disabled={disabled}
          title={title}
        >
          <Download className="size-3" aria-hidden />
          {label}
        </button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Export CSV</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-1.5">
          <label
            htmlFor="csv-export-window"
            className="text-[11px] font-medium text-foreground"
          >
            Window
          </label>
          <select
            id="csv-export-window"
            value={selectedWindow}
            onChange={(event) => setSelectedWindow(event.currentTarget.value)}
            className="h-9 rounded-sm border border-border bg-background px-2.5 text-[12px] text-foreground outline-none focus-visible:border-primary focus-visible:ring-1 focus-visible:ring-primary"
          >
            {windowOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          {error ? <p className="text-[11px] text-loss">{error}</p> : null}
        </div>
        <DialogFooter>
          <button
            type="button"
            onClick={onExport}
            className="inline-flex h-8 items-center justify-center rounded-sm border border-primary bg-primary px-3 text-[11px] text-primary-foreground hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={pending || !selectedWindow}
          >
            {pending ? "Exporting" : "Export"}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function downloadBlob(filename: string, blob: Blob) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement("a")
  link.href = url
  link.download = sanitizedCSVFilename(filename)
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function filenameFromDisposition(value: string | null) {
  const match = value?.match(/filename="?([^";]+)"?/i)
  return match?.[1]
}

export function downloadCSV<T>(
  filename: string,
  columns: Array<CSVColumn<T>>,
  rows: Array<T>
) {
  const csv = [
    columns.map((column) => escapeCSVCell(column.header)).join(","),
    ...rows.map((row) =>
      columns.map((column) => escapeCSVCell(column.value(row))).join(",")
    ),
  ].join("\n")
  downloadBlob(filename, new Blob([csv], { type: "text/csv;charset=utf-8" }))
}

export function sanitizedCSVFilename(filename: string) {
  const normalized = filename
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return normalized.endsWith(".csv")
    ? normalized
    : `${normalized || "export"}.csv`
}

function escapeCSVCell(value: CSVValue) {
  if (value === null || value === undefined) {
    return ""
  }
  const text =
    value instanceof Date
      ? value.toISOString()
      : typeof value === "number"
        ? Number.isFinite(value)
          ? String(value)
          : ""
        : String(value)
  return /[",\n\r]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}
