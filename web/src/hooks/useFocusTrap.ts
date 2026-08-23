import { useEffect, type RefObject } from "react";

const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusableIn(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (node) => !node.hasAttribute("disabled") && node.tabIndex !== -1,
  );
}

export function useFocusTrap(
  containerRef: RefObject<HTMLElement | null>,
  active: boolean,
  restoreFocusRef?: RefObject<HTMLElement | null>,
) {
  useEffect(() => {
    if (!active) {
      return;
    }
    const root = containerRef.current;
    if (!root) {
      return;
    }

    const first = focusableIn(root)[0];
    if (first) {
      first.focus();
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Tab" || !root) {
        return;
      }
      const items = focusableIn(root);
      if (items.length === 0) {
        event.preventDefault();
        return;
      }
      const firstItem = items[0];
      const lastItem = items[items.length - 1];
      if (event.shiftKey && document.activeElement === firstItem) {
        event.preventDefault();
        lastItem.focus();
        return;
      }
      if (!event.shiftKey && document.activeElement === lastItem) {
        event.preventDefault();
        firstItem.focus();
      }
    }

    root.addEventListener("keydown", onKeyDown);
    return () => {
      root.removeEventListener("keydown", onKeyDown);
      restoreFocusRef?.current?.focus();
    };
  }, [active, containerRef, restoreFocusRef]);
}
