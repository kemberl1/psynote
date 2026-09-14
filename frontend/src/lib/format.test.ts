import { describe, expect, it } from "vitest";
import { documentTypeLabel, formatDateTimeShort, formatNewsDate } from "./format";

describe("documentTypeLabel", () => {
  it("calls batch a period, not a packet", () => {
    expect(documentTypeLabel("batch")).toBe("Период дневников");
    expect(
      documentTypeLabel("batch", [
        { code: "batch", title: "Пакет дневников", is_active: true },
      ]),
    ).toBe("Период дневников");
  });
});

describe("formatDateTimeShort", () => {
  it("shows local date and time from a UTC timestamp", () => {
    const local = new Date(2026, 7, 14, 15, 42, 0);
    const out = formatDateTimeShort(local.toISOString());
    expect(out).toMatch(/14/);
    expect(out).toMatch(/15[:.]42/);
  });
});

describe("formatNewsDate", () => {
  it("prints a calendar date for older posts", () => {
    expect(formatNewsDate("2026-01-15T10:00:00Z")).toMatch(/15/);
    expect(formatNewsDate("2026-01-15T10:00:00Z")).toMatch(/2026/);
  });
});
