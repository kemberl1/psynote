import { describe, expect, it } from "vitest";
import {
    batchAutoTitle,
    batchDoneTitle,
    batchPendingTitle,
    batchSessionTitle,
    displayHistoryTitle,
    isAutoBatchTitle,
    stripTitleStatus,
    unpackBatchMeta,
    withPendingSuffix,
} from "./historyTitles";

describe("batch titles", () => {
  it("uses Период, not Пакет", () => {
    expect(batchAutoTitle("2026-07-17", "2026-08-14", 29)).toBe(
      "Период · 17.07.2026–14.08.2026 (29 дн.)",
    );
    expect(batchDoneTitle("2026-07-17", "2026-08-14", 29)).toBe(
      "Период · 17.07.2026–14.08.2026 (29 дн.)",
    );
    expect(batchPendingTitle("2026-07-17", "2026-08-14", 29)).toBe(
      "Период · 17.07.2026–14.08.2026 (29 дн.) · Формируется…",
    );
  });

  it("prefers a custom session name", () => {
    expect(batchSessionTitle("2026-07-17", "2026-08-14", 29, " Иванов ")).toBe(
      "Иванов",
    );
    expect(batchSessionTitle("2026-07-17", "2026-08-14", 29, "")).toBe(
      "Период · 17.07.2026–14.08.2026 (29 дн.)",
    );
  });
});

describe("displayHistoryTitle", () => {
  it("rewrites stored Пакет labels for the UI", () => {
    expect(
      displayHistoryTitle("Пакет · 17.07.2026–14.08.2026 (29 дн.)"),
    ).toBe("Период · 17.07.2026–14.08.2026 (29 дн.)");
  });
});

describe("stripTitleStatus / isAutoBatchTitle", () => {
  it("strips generation suffixes", () => {
    expect(
      stripTitleStatus("Период · 17.07.2026–14.08.2026 (29 дн.) · Формируется…"),
    ).toBe("Период · 17.07.2026–14.08.2026 (29 дн.)");
    expect(stripTitleStatus("Иванов · ошибок: 2")).toBe("Иванов");
    expect(withPendingSuffix("Иванов · Формируется…")).toBe(
      "Иванов · Формируется…",
    );
  });

  it("detects auto period titles including legacy Пакет", () => {
    expect(isAutoBatchTitle("Период · 17.07.2026–14.08.2026 (29 дн.)")).toBe(
      true,
    );
    expect(
      isAutoBatchTitle("Пакет · 17.07.2026–14.08.2026 (29 дн.) · Формируется…"),
    ).toBe(true);
    expect(isAutoBatchTitle("Иванов, июль")).toBe(false);
  });
});

describe("unpackBatchMeta", () => {
  it("reads session_title", () => {
    const { meta } = unpackBatchMeta({
      __batch_meta__: {
        admission_date: "2026-07-01",
        date_from: "2026-07-17",
        date_to: "2026-08-14",
        session_title: "Иванов",
      },
    });
    expect(meta?.session_title).toBe("Иванов");
  });
});
