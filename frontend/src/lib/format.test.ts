import { describe, expect, it } from "vitest";
import { documentTypeLabel, formatDateTimeShort } from "./format";

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
