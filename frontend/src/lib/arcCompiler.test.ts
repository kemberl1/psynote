import { describe, expect, it } from "vitest";
import {
    compileArc,
    extractDatedSnippets,
    extractFacts,
    inferSpeechLevel,
    weekendDutyNote,
} from "./arcCompiler";
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

  it("does not treat staff supervision as a clinical trait", () => {
    const f = extractFacts(DOCTOR_NARRATIVE);
    const blob = [...f.early, ...f.laterBehaviors, ...f.traits].join(" ");
    expect(blob).not.toMatch(/надзор|под наблюдением/i);
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

  it("gives sheets and shoes to at most two daily days each", () => {
    const daily = briefs.filter((b) => b.role !== "exam");
    const withSheets = daily.filter((b) => b.observations.join(" ").includes("простын"));
    const withShoes = daily.filter((b) => b.observations.join(" ").includes("обув"));
    expect(withSheets.length).toBeGreaterThan(0);
    expect(withShoes.length).toBeGreaterThan(0);
    expect(withSheets.length).toBeLessThanOrEqual(2);
    expect(withShoes.length).toBeLessThanOrEqual(2);
  });

  it("lets the 10-day exam recap field acts as period, not a new episode", () => {
    const exam = briefs.find((b) => b.role === "exam");
    expect(exam?.observations.join(" ")).toMatch(/За период \(не новый эпизод сегодня\)/);
  });

  it("never puts staff supervision into day observations", () => {
    for (const b of briefs) {
      expect(b.observations.join(" ")).not.toMatch(/надзор|под наблюдением/i);
    }
  });

  it("does not instruct a weekend recap on Monday 27.07 when the packet starts that day", () => {
    const firstMon = briefs.find((b) => b.isoDate === "2026-07-27");
    expect(firstMon?.calendar).toBe("monday");
    expect(firstMon?.weekendRecap).toBe(false);
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive", leading_syndrome: "psychomotor_autoaggression" },
      firstMon!.dayNumber,
      days.length,
      firstMon!.isoDate,
      DOCTOR_NARRATIVE,
      "",
      "daily",
      firstMon,
    );
    const arc = String(ans.__arc_context__);
    expect(arc).toMatch(/НЕ пиши «за период выходных дней»/);
    expect(arc).not.toMatch(/Понедельник после сб\/вс ЭТОГО пакета/);
  });

  it("instructs a grounded weekend recap on Monday 03.08 after sat/sun in the packet", () => {
    const secondMon = briefs.find((b) => b.isoDate === "2026-08-03");
    expect(secondMon?.calendar).toBe("monday");
    expect(secondMon?.weekendRecap).toBe(true);
    const ans = buildGenerateAnswers(
      { overall_dynamics: "positive" },
      secondMon!.dayNumber,
      days.length,
      secondMon!.isoDate,
      DOCTOR_NARRATIVE,
      "",
      "daily",
      secondMon,
    );
    const arc = String(ans.__arc_context__);
    expect(arc).toMatch(/Понедельник после сб\/вс ЭТОГО пакета/);
    expect(arc).not.toMatch(/НЕ пиши «за период выходных дней»/);
  });

  it("bans the no-aggression stamp when the leading syndrome is autoaggression", () => {
    const ans = buildGenerateAnswers(
      {
        overall_dynamics: "positive",
        leading_syndrome: "psychomotor_autoaggression",
        diagnosis: "F71.18 Умственная отсталость умеренная",
      },
      briefs[0].dayNumber,
      days.length,
      briefs[0].isoDate,
      DOCTOR_NARRATIVE,
      "",
      "daily",
      briefs[0],
    );
    const arc = String(ans.__arc_context__);
    expect(arc).toMatch(/НЕ пиши штамп «без агрессивных, аутоагрессивных/);
    expect(arc).toMatch(/можно дорисовать быт отделения/i);
    expect(arc).toMatch(/прогулки, визиты/);
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
    expect(sat?.weekendDutyNote).toMatch(/с 1-2\.08/);
    expect(satAns.additional_info).toBe("present");
    expect(String(satAns.additional_info_detail)).toMatch(
      /за период выходных дней с 1-2\.08 под наблюдением дежурного мед персонала/,
    );
    expect(String(satAns.__arc_context__)).toMatch(/Дополнительные сведения о заболевании/);
    expect(sun?.weekendDutyNote).toMatch(/с 1-2\.08/);
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

describe("patient-agnostic locks (not one diagnosis)", () => {
  it("forbids intellectual disability when the diagnosis is not F7x", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: "При поступлении напряжен, плаксив. Начал общаться с детьми.",
      batchAnswers: {
        overall_dynamics: "wavy",
        diagnosis: "F92.8. Другие смешанные расстройства поведения и эмоций",
        final_state: "Собственная речь фразовая. Интеллектуально на уровне возрастной нормы.",
      },
      estimatedDischargeDate: "2026-08-12",
    });
    const ans = buildGenerateAnswers(
      {
        overall_dynamics: "wavy",
        diagnosis: "F92.8. Другие смешанные расстройства поведения и эмоций",
        final_state: "Собственная речь фразовая. Интеллектуально на уровне возрастной нормы.",
      },
      briefs[0].dayNumber,
      days.length,
      briefs[0].isoDate,
      "При поступлении напряжен, плаксив.",
      "2026-08-12",
      "daily",
      briefs[0],
    );
    const arc = String(ans.__arc_context__);
    expect(arc).toMatch(/F92\.8/);
    expect(arc).toMatch(/НЕ умственная отсталость|нет F70/i);
    expect(arc).not.toMatch(/умственная отсталость умеренной/);
    expect(briefs[0].speechLevel).toBe("expanded");
  });

  it("still locks moderate ID when the diagnosis is F71", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: DOCTOR_NARRATIVE,
      batchAnswers: {
        overall_dynamics: "positive",
        diagnosis: "F71.18 Умственная отсталость умеренная",
      },
      estimatedDischargeDate: "",
    });
    const arc = String(
      buildGenerateAnswers(
        { overall_dynamics: "positive", diagnosis: "F71.18 Умственная отсталость умеренная" },
        briefs[0].dayNumber,
        days.length,
        briefs[0].isoDate,
        DOCTOR_NARRATIVE,
        "",
        "daily",
        briefs[0],
      ).__arc_context__,
    );
    expect(arc).toMatch(/умеренной/);
    expect(arc).toMatch(/Не пиши «лёгкую»/);
  });

  it("puts parent-day visits on the Wednesday closest to the narrative, not the weekend", () => {
    const days = packDays();
    const narrative =
      "Состояние с улучшением. При встречах с мамой во время родительских дней был требователен, плаксив, истериоформные реакции.";
    const briefs = compileArc({
      days,
      directorContext: narrative,
      batchAnswers: { overall_dynamics: "wavy", diagnosis: "F92.8" },
      estimatedDischargeDate: "",
    });
    const wed = briefs.find((b) => b.isoDate === "2026-07-29");
    const wed2 = briefs.find((b) => b.isoDate === "2026-08-05");
    const sat = briefs.find((b) => b.isoDate === "2026-08-01");
    const mon = briefs.find((b) => b.isoDate === "2026-07-27");
    const onWeds = [wed, wed2].filter((b) =>
      /мам|родительск|истериоформ/i.test(b?.observations.join(" ") ?? ""),
    );
    expect(onWeds.length).toBe(1);
    expect(sat?.observations.join(" ")).not.toMatch(/истериоформ/i);
    expect(mon?.observations.join(" ")).not.toMatch(/истериоформ/i);
    expect(mon?.forbidden.join(" ")).toMatch(/мам|родительск|истериоформ/i);
  });

  it("pins a parent-day event to the Wednesday nearest an explicit date", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext:
        "Состояние с улучшением. 05.08 в родительский день был требователен, плаксив.",
      batchAnswers: { overall_dynamics: "wavy", diagnosis: "F92.8" },
      estimatedDischargeDate: "",
    });
    const wed = briefs.find((b) => b.isoDate === "2026-08-05");
    const otherWed = briefs.find((b) => b.isoDate === "2026-07-29");
    expect(wed?.observations.join(" ")).toMatch(/требователен|плаксив|родительск/i);
    expect(otherWed?.observations.join(" ")).not.toMatch(/требователен|плаксив/i);
  });

  it("puts an early parent-day mention on the first Wednesday of the packet", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext:
        "В первые дни беспокоен. В родительский день плакал после встречи с мамой.",
      batchAnswers: { overall_dynamics: "wavy", diagnosis: "F92.8" },
      estimatedDischargeDate: "",
    });
    const firstWed = briefs.find((b) => b.isoDate === "2026-07-29");
    const laterWed = briefs.find((b) => b.isoDate === "2026-08-05");
    expect(firstWed?.observations.join(" ")).toMatch(/плакал|мам/i);
    expect(laterWed?.observations.join(" ")).not.toMatch(/плакал/i);
  });

  it("puts an explicit weekday incident on that weekday", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: "В пятницу был конфликт с соседкой по палате.",
      batchAnswers: { overall_dynamics: "stable", diagnosis: "F92.8" },
      estimatedDischargeDate: "",
    });
    const fri = briefs.find((b) => b.isoDate === "2026-07-31");
    const wed = briefs.find((b) => b.isoDate === "2026-07-29");
    expect(fri?.observations.join(" ")).toMatch(/конфликт/i);
    expect(wed?.observations.join(" ")).not.toMatch(/конфликт/i);
  });

  it("puts a dated narrative incident on that calendar day", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: "Состояние стабильно. 01.08 на прогулке упал, ссадина на колене.",
      batchAnswers: { overall_dynamics: "stable", diagnosis: "F92.8" },
      estimatedDischargeDate: "",
    });
    const sat = briefs.find((b) => b.isoDate === "2026-08-01");
    const sun = briefs.find((b) => b.isoDate === "2026-08-02");
    expect(sat?.observations.join(" ")).toMatch(/прогулк|упал|ссадин/i);
    expect(sun?.observations.join(" ")).not.toMatch(/упал|ссадин/i);
  });

  it("places dated medication changes on that calendar day, not the whole pack", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: "Состояние с улучшением. Подбор терапии.",
      batchAnswers: {
        overall_dynamics: "wavy",
        diagnosis: "F92.8",
        key_medications:
          "с 22.07 Алимемазин 10 мг/сут, 30.07 отменен Алимемазин, введен Рисперидон 0,5 мг/сут. 04.08 Рисперидон до 1 мг/сут",
      },
      estimatedDischargeDate: "",
    });
    const jul30 = briefs.find((b) => b.isoDate === "2026-07-30");
    const aug04 = briefs.find((b) => b.isoDate === "2026-08-04");
    const jul28 = briefs.find((b) => b.isoDate === "2026-07-28");
    expect(jul30?.therapyToday).toMatch(/рисперидон|0,5/i);
    expect(aug04?.therapyToday).toMatch(/1 мг/i);
    expect(jul28?.therapyToday).toBeNull();
    const exam = briefs.find((b) => b.role === "exam"); // 29.07
    expect(exam?.therapyToday).toMatch(/алимемазин/i);
    expect(exam?.therapyToday).not.toMatch(/04\.08|1 мг/i);
    expect(exam?.includeFinalState).toBe(false);
  });

  it("does not put discharge final-state on a mid-stay 10-day exam", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: DOCTOR_NARRATIVE,
      batchAnswers: {
        overall_dynamics: "positive",
        final_state: "Договорился с мамой о правилах поведения дома.",
      },
      estimatedDischargeDate: "2026-08-12",
    });
    const exam = briefs.find((b) => b.role === "exam");
    expect(exam?.includeFinalState).toBe(false);
    expect(briefs[briefs.length - 1].includeFinalState).toBe(true);
  });

  it("parses dated snippets from a medication line", () => {
    const spans = extractDatedSnippets(
      "с 21.07 таб. Алимемазин 5 мг, с 22.07 Алимемазин 10 мг/сут, 30.07 Рисперидон 0,5 мг",
      2026,
    );
    expect(spans.map((s) => s.iso)).toEqual([
      "2026-07-21",
      "2026-07-22",
      "2026-07-30",
    ]);
  });

  it("infers phrasal speech from the portrait, not from a previous ID case", () => {
    expect(
      inferSpeechLevel(
        "На вопросы отвечает развернуто. Собственная речь фразовая.",
        "F92.8",
        "",
      ),
    ).toBe("expanded");
    expect(
      inferSpeechLevel(
        "Собственная речь представлена отдельными звукокомплексами.",
        "F71.18",
        "",
      ),
    ).toBe("sounds");
  });
});

describe("weekendDutyNote", () => {
  it("formats same-month Saturday–Sunday as дд-дд.мм", () => {
    expect(weekendDutyNote("2026-08-01", 8)).toBe(
      "за период выходных дней с 1-2.08 под наблюдением дежурного мед персонала.",
    );
    expect(weekendDutyNote("2026-08-02", 9)).toBe(
      "за период выходных дней с 1-2.08 под наблюдением дежурного мед персонала.",
    );
  });

  it("formats a weekend that crosses months", () => {
    expect(weekendDutyNote("2026-10-31", 20)).toBe(
      "за период выходных дней с 31.10-1.11 под наблюдением дежурного мед персонала.",
    );
  });

  it("skips the first three hospital days even on a weekend", () => {
    expect(weekendDutyNote("2026-08-01", 1)).toBeNull();
    expect(weekendDutyNote("2026-08-02", 2)).toBeNull();
    expect(weekendDutyNote("2026-08-02", 3)).toBeNull();
    expect(weekendDutyNote("2026-08-01", 4)).not.toBeNull();
  });

  it("skips weekdays", () => {
    expect(weekendDutyNote("2026-08-03", 10)).toBeNull();
  });

  it("does not instruct the duty formula on admission-weekend days", () => {
    const days = [
      { isoDate: "2026-08-01", dayNumber: 1, documentType: "daily" as const },
      { isoDate: "2026-08-02", dayNumber: 2, documentType: "daily" as const },
      { isoDate: "2026-08-08", dayNumber: 8, documentType: "daily" as const },
    ];
    const briefs = compileArc({
      days,
      directorContext: "Состояние стабильно.",
      batchAnswers: { overall_dynamics: "stable" },
      estimatedDischargeDate: "",
    });
    expect(briefs[0].weekendDutyNote).toBeNull();
    expect(briefs[1].weekendDutyNote).toBeNull();
    expect(briefs[2].weekendDutyNote).toMatch(/с 8-9\.08/);
    const early = buildGenerateAnswers(
      { overall_dynamics: "stable" },
      1,
      3,
      "2026-08-01",
      "Состояние стабильно.",
      "",
      "daily",
      briefs[0],
    );
    expect(early.additional_info).toBeUndefined();
    expect(String(early.__arc_context__)).toMatch(/первых трёх дней/);
    expect(String(early.__arc_context__)).not.toMatch(/напиши РОВНО/);
  });
});
