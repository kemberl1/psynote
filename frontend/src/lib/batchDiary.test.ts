// Юнит-тесты логики пакетной генерации (10-дневное правило, план, answers).
import { describe, expect, it } from "vitest";
import {
  buildBatchPlan,
  buildGenerateAnswers,
  dayNumberFromAdmission,
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
      freeContext: "",
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
      { dynamics: "positive", mood: "even", behavior: "ordered" },
      10,
      "контекст",
      "exam_10d",
    );
    expect(ans.period_dynamics).toBeDefined();
    expect(ans.syndrome).toBe("anxiety_depressive");
    expect(ans.events_detail).toContain("День госпитализации: 10");
    expect(ans.events_detail).toContain("контекст");
  });

  it("omits exam fields for daily", () => {
    const ans = buildGenerateAnswers({}, 3, "", "daily");
    expect(ans.period_dynamics).toBeUndefined();
    expect(ans.dynamics).toBe("no_change");
  });
});
