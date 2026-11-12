const announcementLifetimeMs = 5000;

/**
 * Announces `message` to screen readers politely, from a live region that outlives the
 * component that asked, e.g. a card removed by the action it announces. The region is created
 * empty first, because assistive technology only reads changes of an existing live region.
 */
export function announcePolitely(message: string): void {
  const region = document.createElement("div");
  region.setAttribute("role", "status");
  region.setAttribute("aria-live", "polite");
  region.className = "sr-only";
  document.body.append(region);
  window.setTimeout(() => {
    region.textContent = message;
  }, 0);
  window.setTimeout(() => {
    region.remove();
  }, announcementLifetimeMs);
}
