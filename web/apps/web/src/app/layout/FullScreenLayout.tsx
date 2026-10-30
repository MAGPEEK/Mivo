import { Outlet } from "@tanstack/react-router";

/**
 * Frame for full-screen pages such as the player: no sidebar and no top bar, only the main
 * landmark around the page content.
 */
export function FullScreenLayout() {
  return (
    <main className="min-h-dvh">
      <Outlet />
    </main>
  );
}
