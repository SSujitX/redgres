import { describe, expect, it } from "vitest";
import { isStatusPayload } from "./status";

function validComponents(): unknown[] {
  return [
    { id: "redgres_state", state: "ok" },
    { id: "postgres_direct", state: "unavailable", reason: "unreachable" },
    { id: "pgbouncer", state: "not_configured" },
    { id: "redis", state: "ok" },
    { id: "tool_links", state: "not_configured" },
  ];
}

function validPayload(): Record<string, unknown> {
  return {
    components: validComponents(),
    request_id: "0123456789abcdef0123456789abcdef",
  };
}

describe("isStatusPayload", () => {
  it("accepts the fixed complete status contract", () => {
    expect(isStatusPayload(validPayload())).toBe(true);
  });

  it.each([
    ["empty component list", { ...validPayload(), components: [] }],
    ["missing component", { ...validPayload(), components: validComponents().slice(0, 4) }],
    ["duplicate component", { ...validPayload(), components: validComponents().map((item, index) => (index === 1 ? validComponents()[0] : item)) }],
    ["wrong component order", { ...validPayload(), components: [validComponents()[1], validComponents()[0], ...validComponents().slice(2)] }],
    ["unknown state", { ...validPayload(), components: validComponents().map((item, index) => (index === 0 ? { id: "redgres_state", state: "mystery" } : item)) }],
    ["redgres not configured", { ...validPayload(), components: validComponents().map((item, index) => (index === 0 ? { id: "redgres_state", state: "not_configured" } : item)) }],
    ["PgBouncer not implemented", { ...validPayload(), components: validComponents().map((item, index) => (index === 2 ? { id: "pgbouncer", state: "not_implemented" } : item)) }],
    ["tool links unavailable", { ...validPayload(), components: validComponents().map((item, index) => (index === 4 ? { id: "tool_links", state: "unavailable", reason: "unreachable" } : item)) }],
    [
      "missing unavailable reason",
      { ...validPayload(), components: validComponents().map((item, index) => (index === 1 ? { id: "postgres_direct", state: "unavailable" } : item)) },
    ],
    [
      "extra component field",
      { ...validPayload(), components: validComponents().map((item, index) => index === 0 ? { id: "redgres_state", state: "ok", host: "secret" } : item) },
    ],
    ["missing request ID", { components: validComponents() }],
    ["invalid request ID", { ...validPayload(), request_id: "not-lowercase-hex" }],
    ["extra top-level field", { ...validPayload(), host: "secret" }],
  ])("rejects a malformed %s envelope", (_name, payload) => {
    expect(isStatusPayload(payload)).toBe(false);
  });
});
