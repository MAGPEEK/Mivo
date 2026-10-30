// ESLint flat config for the web workspace. Rules and thresholds come from ADR 0019.
import js from "@eslint/js";
import { createTypeScriptImportResolver } from "eslint-import-resolver-typescript";
import { importX } from "eslint-plugin-import-x";
import jsxA11y from "eslint-plugin-jsx-a11y";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import sonarjs from "eslint-plugin-sonarjs";
import { defineConfig, globalIgnores } from "eslint/config";
import globals from "globals";
import { configs as typeScriptConfigs } from "typescript-eslint";

const maxCognitiveComplexity = 15;
const maxLinesPerFunction = 50;
const maxParams = 4;

const sourceFiles = ["**/*.{ts,tsx,js,mjs,cjs,jsx}"];
const typeScriptFiles = ["**/*.{ts,tsx}"];
const reactFiles = ["**/*.{tsx,jsx}"];
const plainJavaScriptFiles = ["**/*.{js,mjs,cjs,jsx}"];
const testFiles = ["**/*.{test,spec}.{ts,tsx}", "**/test/**", "**/e2e/**"];

// Matches #rgb, #rgba, #rrggbb and #rrggbbaa when not followed by more word characters, so
// fragments like "#section-2" stay allowed. Colours belong in packages/ui-tokens (ADR 0011).
const hexColour = String.raw`/#(?:[0-9A-Fa-f]{3,4}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})(?![\w-])/`;
const hexColourMessage =
  "Hex colour literals are only allowed in packages/ui-tokens. Use a design token instead.";

export default defineConfig(
  globalIgnores([
    "**/node_modules/",
    "**/generated/**",
    "**/dist/",
    "**/coverage/",
    "**/playwright-report/",
    "**/test-results/",
  ]),

  {
    files: sourceFiles,
    extends: [js.configs.recommended],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
    },
    linterOptions: {
      reportUnusedDisableDirectives: "error",
    },
  },

  {
    files: typeScriptFiles,
    extends: [typeScriptConfigs.strictTypeChecked, typeScriptConfigs.stylisticTypeChecked],
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/consistent-type-imports": "error",
      "@typescript-eslint/max-params": ["error", { max: maxParams }],
    },
  },

  {
    files: plainJavaScriptFiles,
    extends: [typeScriptConfigs.disableTypeChecked],
    languageOptions: {
      globals: globals.node,
    },
    rules: {
      // TypeScript files use @typescript-eslint/max-params, which ignores `this` parameters.
      "max-params": ["error", { max: maxParams }],
    },
  },

  {
    files: ["**/*.cjs"],
    languageOptions: {
      sourceType: "commonjs",
    },
  },

  {
    files: sourceFiles,
    extends: [importX.flatConfigs.recommended, importX.flatConfigs.typescript],
    settings: {
      "import-x/resolver-next": [
        createTypeScriptImportResolver({
          alwaysTryTypes: true,
          project: ["packages/*/tsconfig.json", "apps/*/tsconfig.json"],
        }),
      ],
    },
    rules: {
      "import-x/no-cycle": "error",
      "import-x/no-self-import": "error",
      "import-x/no-useless-path-segments": "error",
    },
  },

  {
    files: sourceFiles,
    plugins: { sonarjs },
    rules: {
      "sonarjs/cognitive-complexity": ["error", maxCognitiveComplexity],
      "max-lines-per-function": [
        "error",
        { max: maxLinesPerFunction, skipBlankLines: true, skipComments: true, IIFEs: true },
      ],
    },
  },

  {
    files: reactFiles,
    extends: [
      react.configs.flat.recommended,
      react.configs.flat["jsx-runtime"],
      reactHooks.configs.flat.recommended,
      jsxA11y.flatConfigs.strict,
    ],
    languageOptions: {
      globals: globals.browser,
    },
    settings: {
      // Explicit, because react is not installed at the workspace root (pnpm isolation).
      react: { version: "19.2" },
    },
  },

  {
    files: sourceFiles,
    ignores: ["packages/ui-tokens/**"],
    rules: {
      "no-restricted-syntax": [
        "error",
        { selector: `Literal[value=${hexColour}]`, message: hexColourMessage },
        { selector: `TemplateElement[value.raw=${hexColour}]`, message: hexColourMessage },
        { selector: `JSXText[value=${hexColour}]`, message: hexColourMessage },
      ],
    },
  },

  {
    // Tests are allowed long describe/it callbacks; every other rule still applies.
    files: testFiles,
    rules: {
      "max-lines-per-function": "off",
    },
  },
);
