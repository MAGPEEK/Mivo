// Layer rules for the web workspace (ADR 0005, ADR 0019). Paths are relative to web/.
// Rules for apps/<app>/src/{app,features,shared} apply as soon as those folders exist.

// React itself and every React-specific package, scoped or not: react, react-dom, react-i18next,
// @types/react, @tanstack/react-query, @radix-ui/react-slot, @testing-library/react, ...
// Matches whether the import resolved through node_modules (including pnpm's .pnpm store) or
// stayed unresolved because the package is not installed. Written as flat alternatives because
// dependency-cruiser rejects nested quantifiers as unsafe regular expressions.
const reactModules = "(^|node_modules/)(react|react-[^/]+|@[^/]+/react|@[^/]+/react-[^/]+)(/|$)";

/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: "no-circular",
      comment: "Circular dependencies make modules impossible to understand and test alone.",
      severity: "error",
      from: {},
      to: { circular: true },
    },
    {
      name: "not-to-unresolvable",
      comment: "Every import must resolve to a file or an installed package.",
      severity: "error",
      from: {},
      to: { couldNotResolve: true },
    },
    {
      name: "packages-not-to-apps",
      comment: "Shared packages are consumed by apps and never depend on them.",
      severity: "error",
      from: { path: "^packages/" },
      to: { path: "^apps/" },
    },
    {
      name: "apps-not-to-other-apps",
      comment: "Apps share code only through packages/*.",
      severity: "error",
      from: { path: "^apps/([^/]+)/" },
      to: { path: "^apps/", pathNot: "^apps/$1/" },
    },
    {
      name: "framework-free-packages-not-to-react",
      comment:
        "player-core, api-client and ui-tokens stay framework-free so the TV app can reuse them.",
      severity: "error",
      from: { path: "^packages/(player-core|api-client|ui-tokens)/" },
      to: { path: reactModules },
    },
    {
      name: "shared-not-to-features-or-app",
      comment: "src/shared is the lowest app layer and must not know features or the app shell.",
      severity: "error",
      from: { path: "^apps/([^/]+)/src/shared/" },
      to: { path: "^apps/$1/src/(features|app)/" },
    },
    {
      name: "features-not-to-other-feature-internals",
      comment: "A feature may use another feature only through its public index file.",
      severity: "error",
      from: { path: "^apps/([^/]+)/src/features/([^/]+)/" },
      to: {
        path: "^apps/$1/src/features/[^/]+/",
        pathNot: ["^apps/$1/src/features/$2/", "^apps/$1/src/features/[^/]+/index\\.tsx?$"],
      },
    },
    {
      name: "generated-client-only-through-entry",
      comment: "Generated API code is reached only through the @mivo/api-client entry point.",
      severity: "error",
      from: { pathNot: "^packages/api-client/" },
      to: { path: "^packages/api-client/src/generated/" },
    },
  ],
  options: {
    doNotFollow: { path: "node_modules" },
    exclude: {
      path: "(^|/)(dist|coverage|playwright-report|test-results)/",
    },
    // Type-only imports count too: a type import of react is still a framework dependency.
    tsPreCompilationDeps: true,
    combinedDependencies: false,
    enhancedResolveOptions: {
      exportsFields: ["exports"],
      conditionNames: ["import", "require", "node", "default", "types"],
      mainFields: ["module", "main", "types", "typings"],
    },
    reporterOptions: {
      text: { highlightFocused: true },
    },
  },
};
