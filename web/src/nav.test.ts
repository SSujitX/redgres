import { describe, expect, it } from "vitest";
import { filterDocs, lookupDoc, visibleNavEntries } from "./nav";

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

  it("keeps Domain & Network and Expert tools visible in the System group", () => {
    const ids = visibleNavEntries("overview").map((entry) => entry.id);
    expect(ids).toContain("system");
    expect(ids).toContain("domain");
    expect(ids).toContain("tools");
  });
});

describe("filterDocs", () => {
  it("returns catalog hits by title or keyword and fail-closes unknown ids", () => {
    expect(filterDocs("palette").map((hit) => hit.id)).toEqual(["using-search"]);
    expect(filterDocs("inspect").map((hit) => hit.id)).toEqual(["postgres-databases"]);
    expect(filterDocs("Using search").map((hit) => hit.title)).toEqual(["Using search"]);
    expect(filterDocs("documentation")).toEqual([]);
    expect(filterDocs("not-an-article-zzzz")).toEqual([]);
    expect(filterDocs("")).toEqual([]);
    expect(lookupDoc("using-search")?.title).toBe("Using search");
    expect(lookupDoc("not-an-article")).toBeUndefined();
    expect(lookupDoc("docs")).toBeUndefined();
  });
});
