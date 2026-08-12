// Юнит-тесты логики пакетной генерации (10-дневное правило, план, answers).
import { describe, expect, it } from "vitest";
import {
  buildBatchPlan,
  buildGenerateAnswers,
  dayNumberFromAdmission,
  filterDirectorContextForDay,
  isExam10Day,
  resolveDocType,
  validateBatchDates,
} from "./batchDiary";

function d(iso: string): Date {
  const [y, m, day] = iso.split("-").map(Number);
  return new Date(y, m - 1, day);
}

describe("dayNumberFromAdmission", () => {
  it("counts admission day as 1", () => {
    const adm = d("2025-06-01");
    expect(dayNumberFromAdmission(adm, adm)).toBe(1);
  });

  it("counts day 10 correctly", () => {
    const adm = d("2025-06-01");
    expect(dayNumberFromAdmission(adm, d("2025-06-10"))).toBe(10);
  });
});

describe("isExam10Day / resolveDocType", () => {
  const adm = d("2025-06-01");

  it("day 1 is daily", () => {
    expect(isExam10Day(adm, d("2025-06-01"))).toBe(false);
    expect(resolveDocType(adm, d("2025-06-01"))).toBe("daily");
  });

  it("days 10, 20, 30 are exam", () => {
    for (const iso of ["2025-06-10", "2025-06-20", "2025-06-30"]) {
      expect(isExam10Day(adm, d(iso))).toBe(true);
      expect(resolveDocType(adm, d(iso))).toBe("exam_10d");
    }
  });

  it("day 11 is daily", () => {
    expect(isExam10Day(adm, d("2025-06-11"))).toBe(false);
    expect(resolveDocType(adm, d("2025-06-11"))).toBe("daily");
  });
});

describe("validateBatchDates", () => {
  it("rejects range before admission", () => {
    const r = validateBatchDates("2025-06-10", "2025-06-01", "2025-06-05");
    expect(r.ok).toBe(false);
  });

  it("accepts valid range", () => {
    const r = validateBatchDates("2025-06-01", "2025-06-01", "2025-06-05");
    expect(r.ok).toBe(true);
  });
});

describe("buildBatchPlan", () => {
  it("counts daily vs exam days", () => {
    const plan = buildBatchPlan({
      admissionDate: "2025-06-01",
      dateFrom: "2025-06-08",
      dateTo: "2025-06-11",
      answers: {},
      directorContext: "",
      estimatedDischargeDate: "",
    });
    expect(plan).not.toBeNull();
    expect(plan!.days).toHaveLength(4);
    expect(plan!.examCount).toBe(1);
    expect(plan!.dailyCount).toBe(3);
    expect(plan!.days[2].documentType).toBe("exam_10d");
  });
});

describe("buildGenerateAnswers", () => {
  it("includes exam defaults for exam_10d", () => {
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive", leading_syndrome: "anxious" },
      10,
      10,
      "2025-06-10",
      "контекст",
      "",
      "exam_10d",
    );
    expect(ans.period_dynamics).toBe("improvement");
    expect(ans.syndrome).toBe("anxious");
  });

  it("omits exam fields for daily", () => {
    const ans = buildGenerateAnswers({}, 3, 10, "2025-06-03", "", "", "daily");
    expect(ans.period_dynamics).toBeUndefined();
    expect(ans.dynamics).toBe("no_change");
  });

  it("injects arc context with period position, not hospitalization/period mixup", () => {
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive" },
      5,
      10,
      "2025-06-05",
      "режиссёрский контекст",
      "2025-06-15",
      "daily",
    );
    const arc = ans.__arc_context__ as string;
    expect(arc.toLowerCase()).toContain("день госпитализации: 5");
    expect(arc.toLowerCase()).toMatch(/день в выбранном периоде/);
  });

  it("includes estimated discharge date in the brief", () => {
    const ans = buildGenerateAnswers(
      {},
      1,
      7,
      "2025-06-01",
      "",
      "2025-06-10",
      "daily",
    );
    const arc = ans.__arc_context__ as string;
    expect(arc).toContain("2025-06-10");
  });

  it("maps positive dynamics to improvement for exam_10d", () => {
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive" },
      10,
      10,
      "2025-06-10",
      "",
      "",
      "exam_10d",
    );
    expect(ans.period_dynamics).toBe("improvement");
  });

  it("maps negative dynamics correctly", () => {
    const ansDaily = buildGenerateAnswers(
      { overall_dynamics: "negative" },
      3,
      10,
      "2025-06-03",
      "",
      "",
      "daily",
    );
    expect(ansDaily.dynamics).toBe("worsening");

    const ansExam = buildGenerateAnswers(
      { overall_dynamics: "negative" },
      10,
      10,
      "2025-06-10",
      "",
      "",
      "exam_10d",
    );
    expect(ansExam.period_dynamics).toBe("no_improvement");
  });

  it("includes improvement pace via compiled phase, not a dumped percent", () => {
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive", improvement_pace: "fast" },
      2,
      10,
      "2025-06-02",
      "",
      "",
      "daily",
    );
    const arc = ans.__arc_context__ as string;
    expect(arc.toLowerCase()).toMatch(/фаза дуги|день в выбранном периоде/);
  });

  it("maps ecg_eeg event to interventions in exam_10d", () => {
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive", notable_events: ["ecg_eeg"] },
      10,
      10,
      "2025-06-10",
      "",
      "",
      "exam_10d",
    );
    const interventions = ans.interventions as string[];
    expect(interventions).toContain("ecg");
    expect(interventions).toContain("eeg");
  });

  it("filters weekend events only to weekend days", () => {
    const satAns = buildGenerateAnswers(
      { overall_dynamics: "positive" },
      7,
      10,
      "2025-06-07",
      "Фон настроения снижен. На выходных искусал губу до крови.",
      "",
      "daily",
    );
    const satArc = satAns.__arc_context__ as string;
    expect(satArc).toContain("искусал губу");

    const monAns = buildGenerateAnswers(
      { overall_dynamics: "positive" },
      9,
      10,
      "2025-06-09",
      "Фон настроения снижен. На выходных искусал губу до крови.",
      "",
      "daily",
    );
    const monArc = monAns.__arc_context__ as string;
    expect(monArc).not.toContain("искусал губу");
  });
});

describe("filterDirectorContextForDay", () => {
  // 2025-06-07 = Saturday (dow=6), 2025-06-08 = Sunday (dow=0),
  // 2025-06-09 = Monday (dow=1)
  const sat = new Date(2025, 5, 7);
  const sun = new Date(2025, 5, 8);
  const mon = new Date(2025, 5, 9);
  const wed = new Date(2025, 5, 11); // Wednesday

  it("includes weekend events on Saturday", () => {
    const ctx = "Общее состояние стабильно. В выходные встретилась с матерью.";
    const result = filterDirectorContextForDay(ctx, sat, 7, 10);
    expect(result).toContain("встретилась с матерью");
  });

  it("excludes weekend events on Monday", () => {
    const ctx = "Общее состояние стабильно. В выходные встретилась с матерью.";
    const result = filterDirectorContextForDay(ctx, mon, 9, 10);
    expect(result).not.toContain("встретилась с матерью");
    expect(result).toContain("Общее состояние стабильно");
  });

  it("includes Sunday events on Sunday", () => {
    const ctx = "В воскресенье был конфликт с соседкой по палате.";
    const result = filterDirectorContextForDay(ctx, sun, 8, 10);
    expect(result).toContain("конфликт");
  });

  it("excludes Sunday events on Wednesday", () => {
    const ctx = "В воскресенье был конфликт с соседкой по палате.";
    const result = filterDirectorContextForDay(ctx, wed, 11, 10);
    expect(result).not.toContain("конфликт");
  });

  it("includes early-period events for day 1–3", () => {
    const ctx = "При поступлении была резко возбуждена.";
    const result1 = filterDirectorContextForDay(ctx, mon, 1, 10);
    expect(result1).toContain("возбуждена");
    const result4 = filterDirectorContextForDay(ctx, wed, 4, 10);
    expect(result4).not.toContain("возбуждена");
  });

  it("includes late events only for last 2 days", () => {
    const ctx = "Перед выпиской тревога нарастает.";
    // day 9 of 10 → last 2 days (days 9, 10)
    const result9 = filterDirectorContextForDay(ctx, mon, 9, 10);
    expect(result9).toContain("тревога нарастает");
    // day 5 of 10 → not last 2 days
    const result5 = filterDirectorContextForDay(ctx, sat, 5, 10);
    expect(result5).not.toContain("тревога нарастает");
  });

  it("keeps background sentences for all days", () => {
    const ctx = "Фон настроения со снижением, лабильна. В выходные встречалась с матерью.";
    const monResult = filterDirectorContextForDay(ctx, mon, 9, 10);
    // Background present even on non-weekend
    expect(monResult).toContain("Фон настроения");
    // Event excluded on non-weekend
    expect(monResult).not.toContain("встречалась с матерью");
  });

  it("returns empty string for empty context", () => {
    expect(filterDirectorContextForDay("", mon, 3, 10)).toBe("");
  });
});
