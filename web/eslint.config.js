// ESLint flat config for the web workspace. Rules and thresholds come from ADR 0019.
import js from "@eslint/js";
import { createTypeScriptImportResolver } from "eslint-import-resolver-typescript";
import { importX } from "eslint-plugin-import-x";
import jsxA11y from "eslint-plugin-jsx-a11y";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import sonarjs from "eslint-plugin-sonarjs";
import { createRequire } from "node:module";
import { defineConfig, globalIgnores } from "eslint/config";
import globals from "globals";
import { configs as typeScriptConfigs } from "typescript-eslint";

// Node 24 strips the types of this plain-constant module, so the guard follows a rename.
import { productName } from "./packages/ui-tokens/src/brand.ts";

const maxCognitiveComplexity = 15;
const maxLinesPerFunction = 50;
const maxParams = 4;

const sourceFiles = ["**/*.{ts,tsx,js,mjs,cjs,jsx}"];
const typeScriptFiles = ["**/*.{ts,tsx}"];
const reactFiles = ["**/*.{tsx,jsx}"];
// Hooks live in .ts files too, so the hook rules cover every app source file.
const appSourceFiles = ["apps/**/*.{ts,tsx}"];
const plainJavaScriptFiles = ["**/*.{js,mjs,cjs,jsx}"];
const testFiles = ["**/*.{test,spec}.{ts,tsx}", "**/test/**", "**/e2e/**"];
const tokenFiles = ["packages/ui-tokens/**"];
const brandFile = "packages/ui-tokens/src/brand.ts";

// eslint-plugin-react cannot detect the version from the workspace root (pnpm isolates react
// in apps/web), so it reads the installed version there and stays in sync on upgrades.
const reactVersion = createRequire(new URL("apps/web/package.json", import.meta.url))(
  "react/package.json",
).version;

// Colours belong in packages/ui-tokens (ADR 0011). Flagged:
// - #rgb, #rgba, #rrggbb and #rrggbbaa at the start of a string or after a CSS delimiter
//   ("[", "(", ":", ",") or a Tailwind arbitrary-value space ("_"), e.g. "#fff",
//   "bg-[#ff6b57]", "color: #fff", "shadow-[0_1px_#fff]", "border-[1px_solid_#f00]".
// - #rrggbb and #rrggbbaa after whitespace, e.g. "1px solid #ff6b57".
// Trade-off: 3- and 4-digit hex after whitespace ("1px solid #fff") stays unflagged, because
// it is indistinguishable from text such as "Episode #123" or "Season #12". The reverse cost
// is that a 6- or 8-digit hex-only word after a space ("Ticket #123456", "#facade") is
// rejected; such text belongs in the locale files anyway. The same holds for a 3- or 4-digit
// hex-only word after a CSS delimiter such as "(" or ",": "Fixes (#123)" and "#101, #102"
// look like CSS colours and are flagged; that text belongs in the locale files, too.
// Fragments such as "/library#add" or "#section-2" stay allowed. "_" may follow a colour
// ("_#fff_inset"), any other word character or "-" may not.
const hexDigits = "[0-9A-Fa-f]";
const anyHexColour = `#(?:${hexDigits}{3,4}|${hexDigits}{6}|${hexDigits}{8})`;
const longHexColour = `#(?:${hexDigits}{6}|${hexDigits}{8})`;
const hexColour = String.raw`/(?:(?:^|[[(:,_]\s*)${anyHexColour}|\s${longHexColour})(?![0-9A-Za-z-])/`;
const hexColourMessage =
  "Hex colour literals are only allowed in packages/ui-tokens. Use a design token instead.";

// The working title may appear in exactly one source file (ADR 0002). Case-sensitive, so the
// npm scope and lowercase ids stay allowed. Escaped, because esquery reads it as a regex.
const productNameLiteral = `/${productName.replace(/[.*+?^${}()|[\]\\/]/g, String.raw`\$&`)}/`;
const productNameMessage = `The product name is configurable. Import productName from @mivo/ui-tokens (${brandFile}) instead.`;

/**
 * Builds no-restricted-syntax entries that reject string, template and JSX text literals
 * matching an esquery regular expression.
 * @param {string} pattern esquery regular expression literal, e.g. "/abc/"
 * @param {string} message error message
 */
function restrictedLiterals(pattern, message) {
  return [
    { selector: `Literal[value=${pattern}]`, message },
    { selector: `TemplateElement[value.raw=${pattern}]`, message },
    { selector: `JSXText[value=${pattern}]`, message },
  ];
}

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
          // The solution config references every project, so one entry covers the workspace.
          project: "tsconfig.json",
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
      jsxA11y.flatConfigs.strict,
    ],
    languageOptions: {
      globals: globals.browser,
    },
    settings: {
      react: { version: reactVersion },
    },
  },

  {
    files: appSourceFiles,
    extends: [reactHooks.configs.flat.recommended],
  },

  // no-restricted-syntax options do not merge across config objects, so each file group lists
  // every restriction that applies to it.
  {
    files: sourceFiles,
    ignores: tokenFiles,
    rules: {
      "no-restricted-syntax": [
        "error",
        ...restrictedLiterals(hexColour, hexColourMessage),
        ...restrictedLiterals(productNameLiteral, productNameMessage),
      ],
    },
  },

  {
    files: tokenFiles,
    ignores: [brandFile],
    rules: {
      "no-restricted-syntax": [
        "error",
        ...restrictedLiterals(productNameLiteral, productNameMessage),
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
