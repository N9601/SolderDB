import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // Sidebar / dark anchor (from logo's gunmetal cylinder)
        gunmetal: {
          950: "#0c0f14",
          900: "#161a21",
          850: "#1c2129",
          800: "#252b35",
          700: "#323844",
          600: "#434b59",
          500: "#5b6470"
        },
        // Workspace / light surface
        canvas: {
          50: "#fafaf9",   // page bg
          100: "#f5f5f4",  // hover bg
          150: "#eeeeec",  // subtle dividers
          200: "#e5e7eb",  // hairline
          300: "#d1d5db"   // stronger border
        },
        ink: {
          900: "#0f1115",  // headings
          700: "#1f2937",  // body
          500: "#4b5563",  // secondary
          400: "#6b7280",  // tertiary
          300: "#9ca3af"   // disabled/placeholder
        },
        // Accent (molten solder from logo)
        copper: {
          50: "#fdf3eb",
          100: "#fbe3cf",
          300: "#f4b87a",
          400: "#ec9a4b",
          500: "#e07a25",
          600: "#c25e15",
          700: "#8f4310"
        },
        // Steel-blue (logo cylinder secondary)
        steel: {
          100: "#e8edf3",
          300: "#a9b6c8",
          500: "#5b6b8a",
          700: "#3d4a63"
        },
        ok: "#16a34a",
        warn: "#d97706",
        danger: "#dc2626"
      },
      fontFamily: {
        mono: ["JetBrains Mono", "ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
        sans: [
          "Inter",
          "ui-sans-serif",
          "system-ui",
          "Segoe UI",
          "Roboto",
          "sans-serif"
        ]
      },
      boxShadow: {
        card: "0 1px 2px rgba(15, 17, 21, 0.04), 0 1px 3px rgba(15, 17, 21, 0.06)",
        cardHover: "0 2px 4px rgba(15, 17, 21, 0.06), 0 8px 16px rgba(15, 17, 21, 0.08)",
        glow: "0 0 0 4px rgba(224, 122, 37, 0.15)",
        copper: "0 4px 16px rgba(224, 122, 37, 0.28)",
        inset: "inset 0 0 0 1px rgba(255,255,255,0.04)"
      },
      keyframes: {
        sparkPulse: {
          "0%, 100%": { opacity: "1", transform: "scale(1)" },
          "50%": { opacity: "0.7", transform: "scale(1.15)" }
        },
        slideUp: {
          "0%": { opacity: "0", transform: "translateY(4px)" },
          "100%": { opacity: "1", transform: "translateY(0)" }
        },
        pulseCopper: {
          "0%": { boxShadow: "0 0 0 0 rgba(224, 122, 37, 0.55)" },
          "100%": { boxShadow: "0 0 0 10px rgba(224, 122, 37, 0)" }
        }
      },
      animation: {
        spark: "sparkPulse 2.4s ease-in-out infinite",
        slideUp: "slideUp 180ms ease-out both",
        pulseCopper: "pulseCopper 700ms ease-out 1"
      }
    }
  },
  plugins: []
} satisfies Config;
