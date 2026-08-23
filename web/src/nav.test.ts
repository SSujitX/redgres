import { describe, expect, it } from "vitest";
import { visibleNavEntries } from "./nav";

describe("visibleNavEntries", () => {
  it("hides nested service items until that service is current", () => {
    const overview = visibleNavEntries("overview").map((entry) => entry.id);
    expect(overview).toContain("postgres");
    expect(overview).toContain("redis");
    expect(overview).not.toContain("postgres-create");
    expect(overview).not.toContain("postgres-security");
    expect(overview).not.toContain("redis-presets");
  });

  it("shows PostgreSQL children only for an active PostgreSQL section", () => {
    expect(visibleNavEntries("postgres").map((entry) => entry.id)).toEqual(
      expect.arrayContaining(["postgres", "postgres-create", "postgres-security"]),
    );
    expect(visibleNavEntries("postgres-create").map((entry) => entry.id)).toContain("postgres-security");
    expect(visibleNavEntries("postgres").map((entry) => entry.id)).not.toContain("redis-presets");
  });
});
