import { cn } from "@/lib/utils"

export function VenueName({
  className,
  iconClassName,
  label,
  venue,
}: {
  className?: string
  iconClassName?: string
  label?: string
  venue: string
}) {
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-2", className)}>
      <VenueLogo className={iconClassName} venue={venue} />
      <span className="min-w-0 truncate">{label ?? formatVenueLabel(venue)}</span>
    </span>
  )
}

export function VenueLogo({
  className,
  venue,
}: {
  className?: string
  venue: string
}) {
  const normalized = normalizeVenue(venue)

  return (
    <span
      className={cn(
        "inline-flex size-4 shrink-0 items-center justify-center overflow-hidden rounded-[3px] border border-border/70 bg-background text-foreground",
        logoSurfaceClass(normalized),
        className
      )}
      title={formatVenueLabel(venue)}
    >
      <LogoMark venue={normalized} />
    </span>
  )
}

export function formatVenueLabel(value: string) {
  if (value.toLowerCase() === "lighter_free") {
    return "Lighter Free"
  }
  if (value.toLowerCase() === "nado_direct") {
    return "Nado Direct"
  }

  return value
    .split(/[_-]/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}

function LogoMark({ venue }: { venue: string }) {
  switch (venue) {
    case "aster":
      return <AsterLogo />
    case "edgex":
      return <EdgeXLogo />
    case "extended":
      return <ExtendedLogo />
    case "grvt":
      return <GrvtLogo />
    case "hyperliquid":
      return <HyperliquidLogo />
    case "lighter":
    case "lighter_free":
      return <LighterLogo />
    case "nado":
    case "nado_direct":
      return <NadoLogo />
    case "pacifica":
      return <PacificaLogo />
    case "variational_omni":
      return <OmniLogo />
    default:
      return <LetterLogo label={venue.slice(0, 2).toUpperCase()} />
  }
}

function normalizeVenue(venue: string) {
  return venue.toLowerCase()
}

function logoSurfaceClass(venue: string) {
  switch (venue) {
    case "aster":
      return "border-white/10 bg-[#050505]"
    case "edgex":
      return "border-white/10 bg-[#121212]"
    case "extended":
      return "border-white/10 bg-[#080808]"
    case "grvt":
      return "border-[#B7F46A]/25 bg-[#B7F46A]"
    case "hyperliquid":
      return "border-white/10 bg-[#0F1A1E]"
    case "lighter":
    case "lighter_free":
      return "border-white/10 bg-[#121218]"
    case "nado":
    case "nado_direct":
      return "border-white/10 bg-[#000000]"
    case "pacifica":
      return "border-[#55C3E9]/20 bg-[#09111B]"
    case "variational_omni":
      return "border-white/10 bg-[#1B2033]"
    default:
      return ""
  }
}

function AsterLogo() {
  return (
    <svg viewBox="0 0 32.2 32" className="size-full" aria-hidden>
      <path d="M9.133 30.44l.75-3.453c.837-3.851-2.108-7.488-6.062-7.488H.385a16.06 16.06 0 0 0 8.748 10.941Z" fill="#FFD29F" />
      <path d="M10.64 31.066A16.01 16.01 0 0 0 16.058 32c7.662 0 14.07-5.347 15.673-12.501H21.249c-4.725 0-8.809 3.283-9.809 7.885l-.8 3.682Z" fill="#FFD29F" />
      <path d="M32.004 17.899c.074-.623.112-1.257.112-1.9C32.116 7.604 25.629.719 17.378.05l-2.251 10.36c-.836 3.851 2.108 7.489 6.062 7.489h10.815Z" fill="#FFD29F" />
      <path d="M15.746 0C7.021.166 0 7.265 0 15.999c0 .642.038 1.276.112 1.9h3.649c4.725 0 8.81-3.284 9.81-7.885L15.745 0Z" fill="#FFD29F" />
    </svg>
  )
}

function EdgeXLogo() {
  return (
    <svg viewBox="0 0 207 180" className="size-[78%]" aria-hidden>
      <path
        d="M206.98 179.25 103.49 0 0 179.25h206.98ZM21.9 141.381 125.342 38 163.15 179.25 21.9 141.381Z"
        fill="#FFFFFF"
      />
    </svg>
  )
}

function ExtendedLogo() {
  return (
    <svg viewBox="0 0 64 64" className="size-[82%]" aria-hidden>
      <path fill="#00BF7F" d="M31.615 15.27a40.28 40.28 0 0 1-25-8.654c5.41 6.89 8.654 15.564 8.654 25s-3.244 18.11-8.654 25c6.89-5.41 15.565-8.654 25-8.654a40.28 40.28 0 0 1 25 8.654c-5.41-6.89-8.654-15.564-8.654-25a40.28 40.28 0 0 1 8.654-25 40.32 40.32 0 0 1-25 8.654Z" />
    </svg>
  )
}

function HyperliquidLogo() {
  return (
    <svg viewBox="0 0 65.3 57" className="h-[74%] w-[88%]" aria-hidden>
      <path fill="#97FCE4" d="M65.3 24c0 21.5-13.2 28.4-20.2 22.3-5.7-5-7.4-15.6-16-16.7C18.2 28.3 17.2 42.8 10 42.8c-8.4 0-10-12.1-10-18.4C0 18 1.8 9.3 8.9 9.3c8.3 0 8.8 12.5 19.2 11.8C38.4 20.4 38.6 7.4 45.4 1.9 51.3-3 65.3 2.2 65.3 24Z" />
    </svg>
  )
}

function GrvtLogo() {
  return (
    <svg viewBox="0 0 48 48" className="size-full" aria-hidden>
      <circle cx="24" cy="24" r="22" fill="#B7F46A" />
      <path
        d="M15.8 27.2c-4.7 0-8-3.2-8-7.7s3.3-7.7 8-7.7h8.7c3.3 0 6 2.5 6 5.8 0 3.2-2.7 5.8-6 5.8h-5.2"
        fill="none"
        stroke="#0B3040"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="4.8"
      />
      <path
        d="M32.2 20.8c4.7 0 8 3.2 8 7.7s-3.3 7.7-8 7.7h-8.7c-3.3 0-6-2.5-6-5.8 0-3.2 2.7-5.8 6-5.8h5.2"
        fill="none"
        stroke="#0B3040"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="4.8"
      />
    </svg>
  )
}

function LighterLogo() {
  return (
    <svg viewBox="0 0 14 26" className="h-[86%] w-[62%]" aria-hidden>
      <path fill="#FFFFFF" d="M5.989 20.388 0 25.498V5.888L5.989.502v19.886Zm7.811.012-5.989 5.098V17.85l5.989-5.37v7.92Z" />
    </svg>
  )
}

function NadoLogo() {
  return (
    <svg viewBox="0 0 100 100" className="size-full" aria-hidden>
      <rect width="100" height="100" fill="#000000" />
      <path fill="#FFFFFF" d="M17 42 53 35l8 8-8 8-37 7-10-6 6-10h5Zm44 1 10-21 10 2 9 16v11L71 86 58 89l-3-10 11-33-5-3Zm-24 24 13-3 32 36H67L38 69l-1-2Zm11-13 15-5-8 25-10 2 3-22Z" />
    </svg>
  )
}

function OmniLogo() {
  return (
    <svg viewBox="0 0 256 256" className="size-full" aria-hidden>
      <rect width="256" height="256" rx="36" fill="#1B2033" />
      <path
        d="M40 180h33.6c10.5 0 17.9-5.3 22.3-15.9l30.3-75.4c7.2-18 19.4-27 36.6-27 20.9 0 36 13.2 45.2 39.7l21.1 60.5c4.2 12 12.4 18.1 24.9 18.1h2v14h-9.2c-18.8 0-31.9-9.1-39.4-27.3l-17.8-43.1c-5.6-13.7-14.4-20.6-26.5-20.6-11.7 0-20.1 6.6-25.2 19.9l-17.1 44.2c-7 17.9-19.6 26.9-37.9 26.9H40v-14Z"
        fill="#4C9AF8"
      />
      <path
        d="M94 180h25.4c10.2 0 17.3-5 21.2-15.1l20.2-52.2c5.7-14.7 15.7-22 30-22 15.9 0 27.2 8.9 34 26.8l16.7 44.1c4.6 12.3 12.1 18.4 22.5 18.4h2v14h-8.1c-16.1 0-27.6-8-34.3-24l-16.8-39.8c-4.3-10.2-10.7-15.3-19.1-15.3-8.8 0-15.1 5.2-18.9 15.5l-14.2 38.7c-6.1 16.6-17.7 24.9-34.8 24.9H94v-14Z"
        fill="#4C9AF8"
      />
    </svg>
  )
}

function PacificaLogo() {
  return (
    <svg viewBox="0 0 2000 2000" className="size-full" aria-hidden>
      <rect width="2000" height="2000" fill="#09111B" />
      <path d="M1000.32 860.22C1017.75 933.726 1065.22 980.106 1140.55 1003.11C1141.93 1003.54 1143.14 1004.4 1144 1005.57C1144.86 1006.73 1145.32 1008.14 1145.32 1009.59C1145.32 1011.04 1144.86 1012.45 1144 1013.62C1143.14 1014.78 1141.93 1015.64 1140.55 1016.07C1106.66 1024.11 1075.64 1041.33 1050.88 1065.83C1026.12 1090.33 1008.59 1121.17 1000.19 1154.97C999.801 1156.4 998.949 1157.67 997.768 1158.57C996.586 1159.47 995.141 1159.96 993.655 1159.96C992.17 1159.96 990.726 1159.47 989.544 1158.57C988.363 1157.67 987.51 1156.4 987.117 1154.97C978.638 1121.95 961.594 1091.75 937.708 1067.42C913.331 1042.57 882.711 1024.75 849.066 1015.83C847.682 1015.4 846.471 1014.54 845.611 1013.38C844.751 1012.21 844.288 1010.8 844.288 1009.35C844.288 1007.9 844.751 1006.49 845.611 1005.33C846.471 1004.16 847.682 1003.3 849.066 1002.87C923.541 981.075 969.314 933.726 987.358 860.22C987.786 858.836 988.646 857.625 989.812 856.766C990.978 855.906 992.388 855.442 993.837 855.442C995.286 855.442 996.696 855.906 997.862 856.766C999.028 857.625 999.888 858.836 1000.32 860.22Z" fill="#55C3E9" />
      <path d="M1413.38 1137.77C1413.38 1280.28 1358.15 1417.24 1259.29 1519.88C1160.42 1622.52 1025.62 1682.84 883.214 1688.16C880.577 1688.28 877.944 1687.86 875.474 1686.93C873.003 1686 870.746 1684.59 868.835 1682.77C866.924 1680.95 865.4 1678.76 864.354 1676.34C863.309 1673.91 862.763 1671.3 862.749 1668.67V1413.15C926.283 1413.14 987.86 1391.17 1037.05 1350.96C1086.24 1310.75 1120.02 1254.77 1132.67 1192.51C1135.53 1177.27 1143.59 1163.5 1155.47 1153.54C1167.35 1143.58 1182.32 1138.05 1197.82 1137.9L1413.38 1137.77Z" fill="#55C3E9" />
      <path d="M862.387 1197.48V1413.15C719.88 1413.15 582.918 1357.92 480.282 1259.06C377.646 1160.19 317.328 1025.39 312.001 882.986C311.887 880.349 312.305 877.717 313.232 875.246C314.159 872.775 315.576 870.517 317.396 868.606C319.217 866.696 321.402 865.172 323.825 864.126C326.248 863.08 328.858 862.534 331.497 862.521H587.012C587.113 926.051 609.188 987.591 649.491 1036.7C689.795 1085.81 745.846 1119.47 808.136 1131.96C823.357 1134.93 837.08 1143.08 846.971 1155.02C856.862 1166.97 862.31 1181.97 862.387 1197.48Z" fill="#55C3E9" />
      <path d="M803.049 862.401H587.376C587.372 719.873 642.62 582.894 741.51 480.255C840.401 377.615 975.231 317.31 1117.66 312.013C1120.29 311.916 1122.92 312.347 1125.38 313.28C1127.84 314.213 1130.1 315.63 1132 317.447C1133.91 319.264 1135.44 321.445 1136.49 323.86C1137.54 326.275 1138.1 328.877 1138.13 331.511V587.026C1074.57 587.021 1012.97 609.001 963.769 649.238C914.571 689.475 880.805 745.491 868.2 807.786C865.363 823.034 857.312 836.821 845.426 846.784C833.54 856.748 818.559 862.269 803.049 862.401Z" fill="#55C3E9" />
      <path d="M1138.01 802.821V587.026C1280.52 586.99 1417.5 642.213 1520.15 741.084C1622.79 839.955 1683.1 974.772 1688.39 1117.19C1688.53 1119.82 1688.12 1122.45 1687.21 1124.92C1686.3 1127.39 1684.89 1129.65 1683.08 1131.56C1681.27 1133.48 1679.09 1135 1676.67 1136.05C1674.26 1137.1 1671.65 1137.64 1669.02 1137.65H1413.38C1413.38 1074.11 1391.4 1012.53 1351.16 963.35C1310.92 914.172 1254.91 880.43 1192.62 867.85C1177.41 864.996 1163.65 856.956 1153.69 845.1C1143.73 833.243 1138.19 818.303 1138.01 802.821Z" fill="#55C3E9" />
    </svg>
  )
}

function LetterLogo({
  label,
  tone = "oklch(0.55 0.15 230)",
}: {
  label: string
  tone?: string
}) {
  return (
    <span
      className="flex size-full items-center justify-center text-[8px] font-semibold leading-none text-background"
      style={{ backgroundColor: tone }}
      aria-hidden
    >
      {label}
    </span>
  )
}
