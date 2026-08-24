import { describe, expect, it } from "vitest";
import { displayText } from "./displayText";

const BIDI_POINTS = [
  "\u200E",
  "\u200F",
  "\u061C",
  "\u202A",
  "\u202B",
  "\u202C",
  "\u202D",
  "\u202E",
  "\u2066",
  "\u2067",
  "\u2068",
  "\u2069",
] as const;

const EXTRA_INVISIBLE = ["\u200B", "\u200D", "\uFEFF", "\u00AD"] as const;

describe("displayText", () => {
  it("replaces the twelve bidi control code points with U+FFFD", () => {
    for (const point of BIDI_POINTS) {
      expect(displayText(`a${point}b`)).toBe("a\uFFFDb");
      expect(displayText(`a${point}b`)).not.toContain(point);
    }
  });

  it("replaces additional format controls rather than deleting them", () => {
    for (const point of EXTRA_INVISIBLE) {
      expect(displayText(`a${point}b`)).toBe("a\uFFFDb");
      expect(displayText(`a${point}b`)).not.toContain(point);
    }
  });

  it("leaves ordinary operator text unchanged", () => {
    expect(displayText("admin")).toBe("admin");
    expect(displayText("owner.login")).toBe("owner.login");
    expect(displayText("2026-08-25T04:11:09.123456789Z")).toBe("2026-08-25T04:11:09.123456789Z");
    expect(displayText("aabbccddeeff00112233445566778899")).toBe("aabbccddeeff00112233445566778899");
  });
});
