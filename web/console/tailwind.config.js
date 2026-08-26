/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["IBM Plex Sans", "system-ui", "-apple-system", "Segoe UI", "sans-serif"],
        mono: ["IBM Plex Mono", "ui-monospace", "SF Mono", "Consolas", "monospace"],
      },
      colors: {
        ink: {
          DEFAULT: "#14181f",
          muted: "#5b6472",
          faint: "#8b93a1",
        },
        surface: {
          DEFAULT: "#ffffff",
          sunken: "#eef0f3",
        },
        line: {
          DEFAULT: "#dde1e7",
          strong: "#c7ccd4",
        },
        accent: {
          DEFAULT: "#2a3f6b",
          strong: "#1c2c4d",
          soft: "#e7ebf3",
        },
        good: {
          DEFAULT: "#1e7f4f",
          soft: "#e4f3ea",
          border: "#b9dfc8",
        },
        bad: {
          DEFAULT: "#a8382f",
          soft: "#fbeae8",
          border: "#eec3be",
        },
      },
      boxShadow: {
        card: "0 1px 2px rgba(20, 24, 31, 0.04), 0 8px 24px -12px rgba(20, 24, 31, 0.18)",
      },
    },
  },
  plugins: [],
};
