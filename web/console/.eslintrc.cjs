// ESLint config for web/console. Enforces apiClient.ts / keyStorage.ts as the
// sole fetch()/localStorage accessors (DESIGN "Enforceable rule", DDD-12/DDD-17
// console-side equivalent of the Go side's exhaustive-linter obligation).
module.exports = {
  root: true,
  env: { browser: true, es2021: true },
  extends: [
    "eslint:recommended",
    "plugin:@typescript-eslint/recommended",
  ],
  parser: "@typescript-eslint/parser",
  parserOptions: {
    ecmaVersion: "latest",
    sourceType: "module",
    ecmaFeatures: { jsx: true },
  },
  plugins: ["@typescript-eslint"],
  ignorePatterns: ["dist", "*.config.ts", "*.config.js"],
  rules: {
    "no-restricted-globals": [
      "error",
      {
        name: "fetch",
        message:
          "fetch() must only be called from apiClient.ts -- the sole permitted caller (DESIGN SA-D4, Core Principle 12).",
      },
      {
        name: "localStorage",
        message:
          "localStorage must only be accessed from keyStorage.ts -- the sole permitted accessor.",
      },
    ],
  },
  overrides: [
    {
      files: ["src/apiClient.ts"],
      rules: {
        "no-restricted-globals": [
          "error",
          {
            name: "localStorage",
            message:
              "localStorage must only be accessed from keyStorage.ts -- the sole permitted accessor.",
          },
        ],
      },
    },
    {
      files: ["src/keyStorage.ts"],
      rules: {
        "no-restricted-globals": [
          "error",
          {
            name: "fetch",
            message:
              "fetch() must only be called from apiClient.ts -- the sole permitted caller (DESIGN SA-D4, Core Principle 12).",
          },
        ],
      },
    },
    {
      files: ["**/*.test.ts", "**/*.test.tsx", "src/setupTests.ts"],
      rules: {
        "no-restricted-globals": "off",
      },
    },
  ],
};
