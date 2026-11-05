import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";

import { findTypeAhead, nextEnabledIndex } from "./menuNavigation";

/** Pause after which typed characters start a new type-ahead search. */
const typeAheadResetMs = 500;

/** Whether the popup opens below or above its button. */
export type MenuPlacement = "down" | "up";

/** Options of {@link useMenu}. */
export interface MenuOptions {
  readonly labels: readonly string[];
  readonly disabled: readonly boolean[];
}

type InitialFocus = "first" | "last";
type ItemRefs = RefObject<(HTMLButtonElement | null)[]>;

/** Element refs of a menu; the component owns them so render code never touches `.current`. */
export interface MenuRefs {
  readonly trigger: RefObject<HTMLButtonElement | null>;
  readonly menu: RefObject<HTMLDivElement | null>;
  readonly items: ItemRefs;
}

// Flips the popup upward when it does not fit below the button but there is more room above.
function usePlacement(
  open: boolean,
  triggerRef: RefObject<HTMLElement | null>,
  menuRef: RefObject<HTMLElement | null>,
): MenuPlacement {
  const [placement, setPlacement] = useState<MenuPlacement>("down");
  useLayoutEffect(() => {
    const trigger = triggerRef.current;
    const menu = menuRef.current;
    if (!open || trigger === null || menu === null) {
      return;
    }
    const rect = trigger.getBoundingClientRect();
    const below = window.innerHeight - rect.bottom;
    setPlacement(below < menu.offsetHeight && rect.top > below ? "up" : "down");
  }, [open, triggerRef, menuRef]);
  return placement;
}

// Closes the menu on a press outside of the button and the popup.
function useOutsidePress(
  open: boolean,
  inside: readonly RefObject<HTMLElement | null>[],
  onOutside: () => void,
): void {
  useEffect(() => {
    if (!open) {
      return;
    }
    // pointerdown covers mouse, pen and touch alike.
    const onPointerDown = (event: Event): void => {
      const target = event.target;
      if (target instanceof Node && !inside.some((ref) => ref.current?.contains(target))) {
        onOutside();
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
    };
    // `inside` and `onOutside` are fresh on every render; the listener only needs the open state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
}

// Collects typed characters until a pause and returns the matching entry index, or -1.
function useTypeAhead({
  labels,
  disabled,
}: MenuOptions): (character: string, from: number) => number {
  const typed = useRef({ text: "", timer: 0 });
  return (character, from) => {
    window.clearTimeout(typed.current.timer);
    typed.current.text += character;
    typed.current.timer = window.setTimeout(() => {
      typed.current.text = "";
    }, typeAheadResetMs);
    return findTypeAhead(labels, disabled, typed.current.text, from);
  };
}

// Index to focus for a navigation key, `undefined` when the key is not one of them.
function navigationTarget(
  key: string,
  disabled: readonly boolean[],
  current: number,
): number | undefined {
  switch (key) {
    case "ArrowDown":
      return nextEnabledIndex(disabled, current, 1);
    case "ArrowUp":
      return nextEnabledIndex(disabled, current, -1);
    case "Home":
      return nextEnabledIndex(disabled, -1, 1);
    case "End":
      return nextEnabledIndex(disabled, 0, -1);
    default:
      return undefined;
  }
}

function isTypeAheadKey(event: KeyboardEvent): boolean {
  const modified = event.ctrlKey || event.metaKey || event.altKey;
  return event.key.length === 1 && event.key !== " " && !modified;
}

// Escape is handled by a native listener on the menu element: modal dialogs listen natively on
// their own element, which runs before React's root listener, so a React stopPropagation would
// come too late and Escape would close the dialog together with the menu.
function useEscapeToClose(
  open: boolean,
  menuRef: RefObject<HTMLElement | null>,
  onEscape: () => void,
): void {
  useEffect(() => {
    const menu = menuRef.current;
    if (!open || menu === null) {
      return;
    }
    const onKeyDown = (event: globalThis.KeyboardEvent): void => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onEscape();
      }
    };
    menu.addEventListener("keydown", onKeyDown);
    return () => {
      menu.removeEventListener("keydown", onKeyDown);
    };
    // `onEscape` is fresh on every render; the listener only needs the open state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, menuRef]);
}

function focusItem(items: ItemRefs, index: number): void {
  items.current[index]?.focus();
}

interface MenuKeyContext {
  readonly disabled: readonly boolean[];
  readonly items: ItemRefs;
  readonly typeAhead: (character: string, from: number) => number;
  readonly close: (returnFocus: boolean) => void;
}

function handleMenuKey(event: KeyboardEvent, context: MenuKeyContext): void {
  const { disabled, items, typeAhead, close } = context;
  const current = items.current.findIndex((item) => item === document.activeElement);
  const target = navigationTarget(event.key, disabled, current);
  if (target !== undefined) {
    event.preventDefault();
    focusItem(items, target);
  } else if (event.key === "Tab") {
    // Tab would move focus out of a menu that is about to unmount; return it to the button.
    event.preventDefault();
    close(true);
  } else if (isTypeAheadKey(event)) {
    focusItem(items, typeAhead(event.key, current));
  }
}

/**
 * State and keyboard behaviour of a menu button: arrow keys, Home/End, type-ahead, Escape,
 * focus return, outside press and flipping upward when there is no room below.
 */
export function useMenu(options: MenuOptions, refs: MenuRefs) {
  const { disabled } = options;
  const [open, setOpen] = useState(false);
  const initialFocus = useRef<InitialFocus>("first");
  const placement = usePlacement(open, refs.trigger, refs.menu);
  const typeAhead = useTypeAhead(options);
  const close = (returnFocus: boolean): void => {
    setOpen(false);
    if (returnFocus) {
      refs.trigger.current?.focus();
    }
  };
  const openWith = (focus: InitialFocus): void => {
    initialFocus.current = focus;
    setOpen(true);
  };
  useOutsidePress(open, [refs.trigger, refs.menu], () => {
    setOpen(false);
  });
  useEscapeToClose(open, refs.menu, () => {
    close(true);
  });
  useEffect(() => {
    if (open) {
      const first = initialFocus.current === "first";
      focusItem(refs.items, nextEnabledIndex(disabled, first ? -1 : 0, first ? 1 : -1));
    }
    // Focus moves once per opening; later changes of `disabled` must not steal it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  return {
    open,
    placement,
    close,
    toggle: (): void => {
      toggleMenu({ open, openWith, close });
    },
    onTriggerKeyDown: (event: KeyboardEvent): void => {
      handleTriggerKey(event, openWith);
    },
    onMenuKeyDown: (event: KeyboardEvent): void => {
      handleMenuKey(event, { disabled, items: refs.items, typeAhead, close });
    },
  };
}

interface MenuControls {
  readonly open: boolean;
  readonly openWith: (focus: InitialFocus) => void;
  readonly close: (returnFocus: boolean) => void;
}

// A click on the menu button.
function toggleMenu({ open, openWith, close }: MenuControls): void {
  if (open) {
    close(false);
  } else {
    openWith("first");
  }
}

// ArrowDown/ArrowUp on the menu button open the menu on its first/last entry.
function handleTriggerKey(event: KeyboardEvent, openWith: MenuControls["openWith"]): void {
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    openWith(event.key === "ArrowDown" ? "first" : "last");
  }
}
