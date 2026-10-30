import { Outlet } from "@tanstack/react-router";

/** Frame around every page. The sidebar and top bar join it in M1-15. */
export function RootLayout() {
  return (
    <main className="min-h-dvh px-6 py-8">
      <Outlet />
    </main>
  );
}
