/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
    "./pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
    "./lib/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        obsidian: {
          DEFAULT: "#090A0F",
          950: "#06070B",
          900: "#090A0F",
          850: "#0E111A",
          800: "#131724",
          750: "#1A2030",
          700: "#222B40",
          600: "#323F5C",
        },
        fin: {
          emerald: "#10B981",
          amber: "#F59E0B",
          rose: "#F43F5E",
          cyan: "#06B6D4",
          purple: "#8B5CF6",
        },
        brand: {
          50: "#f0fdf4",
          500: "#10B981",
          600: "#059669",
          900: "#064e3b",
        },
        bone: {
          DEFAULT: "#FAF9F6",
          50: "#FAF9F6",
          100: "#F5F4F0",
          200: "#EBE9E2",
        },
        maroon: {
          DEFAULT: "#7A1C24",
          900: "#4A0E13",
          800: "#7A1C24",
          700: "#A82833",
        },
      },
      fontFamily: {
        sans: [
          "Inter",
          "Geist Sans",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "sans-serif",
        ],
        mono: [
          "Geist Mono",
          "Fira Code",
          "JetBrains Mono",
          "ui-monospace",
          "monospace",
        ],
      },
      borderColor: {
        hairline: "rgba(255, 255, 255, 0.07)",
      },
      boxShadow: {
        glow: "0 0 15px -3px rgba(16, 185, 129, 0.15)",
        "glow-rose": "0 0 15px -3px rgba(244, 63, 94, 0.15)",
        "glow-amber": "0 0 15px -3px rgba(245, 158, 11, 0.15)",
      },
    },
  },
  plugins: [],
};
