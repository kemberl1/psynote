import { describe, expect, it } from "vitest";
import {
  applyDiaryStamp,
  buildExportSubstitutions,
  diaryDateFromTitle,
  formatExportDate,
} from "./exportSubstitutions";

describe("diary date stamp", () => {
  it("reads the examination day from a batch title", () => {
    expect(diaryDateFromTitle("День 8 · 27.07.2026 · Ежедневный осмотр")).toBe(
      "27.07.2026",
    );
  });

  it("formats ISO day without UTC shift", () => {
    expect(formatExportDate("2026-07-27")).toBe("27.07.2026");
  });

  it("always stamps 10:00 and prefers title date over created_at", () => {
    const subs = buildExportSubstitutions({
      title: "День 11 · 30.07.2026 · Ежедневный осмотр",
      createdAt: "2026-08-13T08:00:39.15752Z",
      doctorName: "Врач Т.",
    });
    expect(subs["[ДАТА]"]).toBe("30.07.2026");
    expect(subs["[ВРЕМЯ]"]).toBe("10:00");
    expect(subs["[ФИО_ВРАЧА]"]).toBe("Врач Т.");
  });

  it("fills placeholders in preview text", () => {
    const out = applyDiaryStamp("[ДАТА] время: [ВРЕМЯ]", {
      title: "День 8 · 27.07.2026 · Ежедневный осмотр",
    });
    expect(out).toBe("27.07.2026 время: 10:00");
  });
});
