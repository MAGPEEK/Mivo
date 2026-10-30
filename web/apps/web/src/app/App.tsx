import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import type { i18n as I18n } from "i18next";
import { I18nextProvider } from "react-i18next";

import { useDocumentLanguage } from "../shared/i18n";
import type { AppRouter } from "./router";

/** Long-lived services the app runs with. `main.tsx` creates them; tests pass their own. */
export interface AppProps {
  readonly i18n: I18n;
  readonly queryClient: QueryClient;
  readonly router: AppRouter;
}

/** Root component: provides translations, server state and routing. */
export function App({ i18n, queryClient, router }: AppProps) {
  useDocumentLanguage(i18n);
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  );
}
