import { describe, expect, it } from "vitest";
import { compileArc, extractFacts } from "./arcCompiler";
import { buildGenerateAnswers } from "./batchDiary";

const DOCTOR_NARRATIVE =
  "Психическое состояние с улучшением в отделении. Режимные моменты полностью не мог осмыслить по причине выраженного когнитивного дефекта. Находился под постоянным надзором медперсонала. В первые дни был двигательно суетлив, периодами расторможен, в дальнейшем в течение дней преимущественно бездеятелен, хаотично перемещается по палате, стаскивает простыни с кроватей других детей, собирает обувь, при вербальной коррекции раздражается, начинает капризничать, плакать. Фон настроения неустойчив. С детьми по палате практически не взаимодействует. Гигиенические навыки сформированы недостаточно, ест самостоятельно, пользуется ложкой. Проводился подбор лекарственной терапии, с целью купирования возбуждений с агрессией в терапии был продолжен прием таб. Левомепромазина с коррекцией дозировки до 112,5 мг/сут, а таб. Риперидон в дозировке до 2 мг/сут был с постепенным снижением и отмены. Также продолжен прием противоэпилептической терапии таб. Вальпроевой кислоты в дозировке 750 мг/сут по рекомендации невролога. На фоне коррекции терапии с положительной динамикой, стал более упорядоченным в поведении, настроение постепенно приблизилось к ровному, снизилась частота возбуждений. Остается трудным в поведении, нуждается в индивидуальном подходе. Аппетит и сон достаточные.";

function packDays() {
  const start = new Date(2026, 6, 27); // 27.07.2026, admission day 8
  return Array.from({ length: 12 }, (_, i) => {
    const d = new Date(start);
    d.setDate(start.getDate() + i);
    const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    const dayNumber = 8 + i;
    return {
      isoDate: iso,
      dayNumber,
      documentType: (dayNumber % 10 === 0 ? "exam_10d" : "daily") as "daily" | "exam_10d",
    };
  });
}

describe("extractFacts", () => {
  it("splits first-days vs later field behavior", () => {
    const f = extractFacts(DOCTOR_NARRATIVE);
    expect(f.early.join(" ")).toMatch(/суетлив|растормож/i);
    expect(f.laterBehaviors.join(" ")).toMatch(/простын/i);
    expect(f.laterBehaviors.join(" ")).toMatch(/обув/i);
    expect(f.early.join(" ")).not.toMatch(/простын/i);
  });

  it("keeps therapy out of the daily behavior pool", () => {
    const f = extractFacts(DOCTOR_NARRATIVE);
    expect(f.therapy.join(" ")).toMatch(/левомепромазин/i);
    expect(f.laterBehaviors.join(" ")).not.toMatch(/112,5/i);
  });
});

describe("compileArc", () => {
  const days = packDays();
  const briefs = compileArc({
    days,
    directorContext: DOCTOR_NARRATIVE,
    batchAnswers: {
      overall_dynamics: "positive",
      improvement_pace: "moderate",
      leading_syndrome: "psychomotor_autoaggression",
      final_state: "При контакте с врачом берет его за руку, ведет к двери палаты.",
    },
    estimatedDischargeDate: "",
  });

  it("does not treat day 8 of a 12-day pack as 67% done", () => {
    expect(briefs[0].periodPct).toBe(0);
    expect(briefs[0].phase).not.toBe("residual");
    expect(briefs[briefs.length - 1].periodPct).toBe(100);
  });

  it("does not put final-state door-leading on early days", () => {
    const early = briefs.filter((b) => b.role !== "exam").slice(0, 4);
    for (const b of early) {
      expect(b.includeFinalState).toBe(false);
      expect(b.forbidden.join(" ")).toMatch(/ведет к двери/i);
    }
  });

  it("rotates sheet/shoes observations instead of pasting both every day", () => {
    const daily = briefs.filter((b) => b.role !== "exam");
    const withSheets = daily.filter((b) => b.observations.join(" ").includes("простын"));
    const withShoes = daily.filter((b) => b.observations.join(" ").includes("обув"));
    expect(withSheets.length).toBeGreaterThan(0);
    expect(withShoes.length).toBeGreaterThan(0);
    expect(withSheets.length).toBeLessThan(daily.length);
    expect(withShoes.length).toBeLessThan(daily.length);
  });

  it("mentions therapy on exam or titration day, not on every daily", () => {
    const withRx = briefs.filter((b) => b.therapyToday);
    expect(withRx.length).toBeGreaterThanOrEqual(1);
    expect(withRx.length).toBeLessThan(briefs.length);
    expect(briefs.some((b) => b.role === "exam" && b.therapyToday)).toBe(true);
  });

  it("interpolates mood from unstable toward even", () => {
    expect(briefs[0].mood).toBe("unstable");
    expect(briefs[briefs.length - 1].mood).toBe("even");
  });

  it("does not put sheet/shoe field acts on residual last days", () => {
    const lastDaily = briefs.filter((b) => b.role !== "exam").slice(-2);
    for (const b of lastDaily) {
      expect(b.phase).toBe("residual");
      expect(b.observations.join(" ")).not.toMatch(/простын|обув/i);
      expect(b.forbidden.join(" ")).toMatch(/простын|обув/i);
    }
  });

  it("marks Saturday/Sunday as weekend in the brief", () => {
    const sat = briefs.find((b) => b.isoDate === "2026-08-01");
    const sun = briefs.find((b) => b.isoDate === "2026-08-02");
    expect(sat?.calendar).toBe("saturday");
    expect(sun?.calendar).toBe("sunday");
    const satAns = buildGenerateAnswers(
      { overall_dynamics: "positive", diagnosis: "F71.18" },
      sat!.dayNumber,
      days.length,
      sat!.isoDate,
      DOCTOR_NARRATIVE,
      "",
      "daily",
      sat,
    );
    expect(String(satAns.__arc_context__)).toMatch(/суббота|выходн/i);
  });

  it("locks the ICD diagnosis into every day brief", () => {
    const withDx = compileArc({
      days,
      directorContext: DOCTOR_NARRATIVE,
      batchAnswers: {
        overall_dynamics: "positive",
        diagnosis: "F71.18 Умственная отсталость умеренная",
        final_state: "берет за руку",
      },
      estimatedDischargeDate: "",
    });
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive", diagnosis: "F71.18 Умственная отсталость умеренная" },
      withDx[0].dayNumber,
      days.length,
      withDx[0].isoDate,
      DOCTOR_NARRATIVE,
      "",
      "daily",
      withDx[0],
    );
    expect(ans.diagnosis).toMatch(/F71\.18/);
    expect(String(ans.__arc_context__)).toMatch(/F71\.18/);
    expect(String(ans.__arc_context__)).toMatch(/не подставляй другой код/i);
  });

  it("does not hardcode polite productive contact", () => {
    for (const b of briefs) {
      expect(b.contact).not.toContain("polite_staff");
      expect(b.contact).not.toContain("productive");
    }
  });

  it("locks nonverbal speech so days cannot get verbal cliches", () => {
    const withSpeech = compileArc({
      days,
      directorContext: DOCTOR_NARRATIVE + " Собственная речь представлена отдельными звукокомплексами.",
      batchAnswers: {
        overall_dynamics: "positive",
        diagnosis: "F71.18 Умственная отсталость умеренная",
        final_state: "берет за руку, ведет к двери. Речь — звукокомплексы.",
      },
      estimatedDischargeDate: "",
    });
    expect(withSpeech[0].speechLevel).toBe("sounds");
    expect(withSpeech[0].contact).not.toContain("does_not_disclose");
    const ans = buildGenerateAnswers(
      {
        overall_dynamics: "positive",
        diagnosis: "F71.18 Умственная отсталость умеренная",
        final_state: "берет за руку. Речь — звукокомплексы.",
      },
      withSpeech[0].dayNumber,
      days.length,
      withSpeech[0].isoDate,
      DOCTOR_NARRATIVE + " звукокомплексами",
      "",
      "daily",
      withSpeech[0],
    );
    const arc = String(ans.__arc_context__);
    expect(arc).toMatch(/звукокомплекс/i);
    expect(arc).toMatch(/ЗАПРЕЩЕНО/);
    expect(arc).toMatch(/односложн/i);
    expect(arc).toMatch(/без формулы «под наблюдением персонала»|не пиши.*под наблюдением/i);
    expect(ans.complaints).toBeUndefined();
  });
});

describe("buildGenerateAnswers with compiled briefs", () => {
  it("puts a day-slice brief into __arc_context__, not the whole epicrisis", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: DOCTOR_NARRATIVE,
      batchAnswers: { overall_dynamics: "positive", final_state: "берет за руку" },
      estimatedDischargeDate: "",
    });
    const first = buildGenerateAnswers(
      { overall_dynamics: "positive", final_state: "берет за руку" },
      days[0].dayNumber,
      days.length,
      days[0].isoDate,
      DOCTOR_NARRATIVE,
      "",
      "daily",
      briefs[0],
    );
    const arc = String(first.__arc_context__);
    expect(arc).toMatch(/День в выбранном периоде: 1 из 12/);
    expect(arc).not.toMatch(/67%/);
    expect(arc.toLowerCase()).toMatch(/не пиши|не цитируй|не упоминай|не повторяй/);
    expect(first.mood).toBe("unstable");
  });

  it("keeps weekend incidents off weekdays", () => {
    const mon = buildGenerateAnswers(
      { overall_dynamics: "positive" },
      9,
      10,
      "2025-06-09",
      "Фон настроения снижен. На выходных искусал губу до крови.",
      "",
      "daily",
    );
    const arc = String(mon.__arc_context__ ?? "");
    expect(arc).not.toContain("искусал губу");
  });
});
