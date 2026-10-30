import "@fontsource-variable/inter";
import "./styles.css";

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
      i18n={createI18n(navigator.languages)}
      queryClient={new QueryClient()}
      router={createAppRouter()}
    />
  </StrictMode>,
);
