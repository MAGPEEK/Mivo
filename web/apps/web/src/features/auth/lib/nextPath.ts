import { isAuthFlowPath } from "./authPaths";

// A placeholder origin to resolve the value against; only its identity matters, it is never
// contacted. ".invalid" is reserved (RFC 6761), so it can never be a real host.
const placeholderOrigin = "http://next.invalid";

// Longer values are not pages of this app but pasted junk or an attack on the URL bar.
const maxNextPathLength = 2048;

// Control characters, white space and backslashes: browsers strip tabs and newlines before
// parsing and read "\" as "/", which can turn "/\t/evil" or "/\evil" into "//evil".
const lastControlOrSpaceCode = 0x20;
const firstDeleteCode = 0x7f;
const lastC1ControlCode = 0x9f;

function isUnsafeCharacter(character: string): boolean {
  const code = character.codePointAt(0) ?? 0;
  return (
    character === "\\" ||
    code <= lastControlOrSpaceCode ||
    (code >= firstDeleteCode && code <= lastC1ControlCode)
  );
}

/**
 * Returns `value` when it is a path of this app that a visitor may be sent to after sign-in,
 * otherwise `undefined`. Only same-app relative paths pass: anything that a browser could read
 * as another origin (`//evil`, `/\evil`, `https://…`, `javascript:…`) is refused, so `next`
 * can never redirect elsewhere (no open redirect). The sign-in flow pages themselves are
 * refused too, so a return never loops.
 */
export function sanitizeNextPath(value: unknown): string | undefined {
  if (typeof value !== "string" || !isRelativeAppPath(value)) {
    return undefined;
  }
  let url: URL;
  try {
    url = new URL(value, placeholderOrigin);
  } catch {
    return undefined;
  }
  if (url.origin !== placeholderOrigin || isAuthFlowPath(url.pathname)) {
    return undefined;
  }
  return `${url.pathname}${url.search}${url.hash}`;
}

function isRelativeAppPath(value: string): boolean {
  return (
    value.length <= maxNextPathLength &&
    value.startsWith("/") &&
    !value.startsWith("//") &&
    !Array.from(value).some(isUnsafeCharacter)
  );
}

/**
 * The `next` value for the page at `href`: the page itself, or `undefined` for Home and the
 * sign-in flow pages, where returning adds nothing.
 */
export function nextPathFor(href: string): string | undefined {
  const next = sanitizeNextPath(href);
  return next === "/" ? undefined : next;
}
