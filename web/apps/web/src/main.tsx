import "@fontsource-variable/inter";
import "./styles.css";

import { createApiClient } from "@mivo/api-client";
import { QueryClient } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import { createAppRouter } from "./app/router";
import { createI18n } from "./shared/i18n";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element");
}

createRoot(rootElement).render(
  <StrictMode>
    <App
      apiClient={createApiClient()}
      i18n={createI18n(navigator.languages)}
      queryClient={new QueryClient()}
      router={createAppRouter()}
    />
  </StrictMode>,
);
