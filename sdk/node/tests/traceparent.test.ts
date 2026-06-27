// sdk/node/tests/traceparent.test.ts
import { describe, it, expect } from "vitest";
import { parseTraceparent, formatTraceparent } from "../src/traceparent.js";

describe("parseTraceparent", () => {
  it("parses a valid header", () => {
    const result = parseTraceparent(
      "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
    );
    expect(result).toEqual({
      traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
      parentId: "00f067aa0ba902b7",
    });
  });

  it("returns null for wrong number of segments", () => {
    expect(parseTraceparent("bad")).toBeNull();
    expect(
      parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7")
    ).toBeNull();
  });

  it("returns null for unsupported version", () => {
    expect(
      parseTraceparent("01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
    ).toBeNull();
  });

  it("returns null for wrong field lengths", () => {
    expect(parseTraceparent("00-tooshort-00f067aa0ba902b7-01")).toBeNull();
    expect(
      parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-short-01")
    ).toBeNull();
    expect(
      parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-1")
    ).toBeNull();
  });

  it("returns null for uppercase hex", () => {
    expect(
      parseTraceparent("00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01")
    ).toBeNull();
  });

  it("returns null for all-zero trace-id or parent-id", () => {
    expect(
      parseTraceparent(
        "00-00000000000000000000000000000000-00f067aa0ba902b7-01"
      )
    ).toBeNull();
    expect(
      parseTraceparent(
        "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01"
      )
    ).toBeNull();
  });
});

describe("formatTraceparent", () => {
  it("formats trace and span ids into W3C header", () => {
    expect(
      formatTraceparent(
        "4bf92f3577b34da6a3ce929d0e0e4736",
        "00f067aa0ba902b7"
      )
    ).toBe("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01");
  });
});
