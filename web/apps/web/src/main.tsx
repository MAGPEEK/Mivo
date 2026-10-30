import "@fontsource-variable/inter";
import "./styles.css";

import { createApiClient } from "@mivo/api-client";
import { QueryClient } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import { createAppI18n } from "./app/i18n";
import { createAppRouter } from "./app/router";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element");
}

createRoot(rootElement).render(
  <StrictMode>
    <App
      apiClient={createApiClient()}
      i18n={createAppI18n(navigator.languages)}
      queryClient={new QueryClient()}
      router={createAppRouter()}
    />
  </StrictMode>,
);
