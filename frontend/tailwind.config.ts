import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        gunmetal: {
          950: "#080a0d",
          900: "#0e1116",
          850: "#141821",
          800: "#1a1f2a",
          700: "#242a36",
          600: "#2f3744",
          500: "#3c4655"
        },
        silver: {
          50: "#f3f5f8",
          100: "#dde2ea",
          200: "#c0c7d1",
          300: "#9aa3b0",
          400: "#7a8492"
        },
        copper: {
          300: "#f4b87a",
          400: "#ec9a4b",
          500: "#e07a25",
          600: "#c25e15",
          700: "#8f4310"
        },
        flux: {
          glow: "#ffb066"
        }
      },
      fontFamily: {
        mono: ["JetBrains Mono", "ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
        sans: ["Inter", "system-ui", "Segoe UI", "Roboto", "sans-serif"]
      },
      boxShadow: {
        "copper-glow": "0 0 12px rgba(236, 154, 75, 0.35)",
        "panel": "inset 0 1px 0 rgba(255,255,255,0.04), 0 1px 0 rgba(0,0,0,0.5)"
      }
    }
  },
  plugins: []
} satisfies Config;
