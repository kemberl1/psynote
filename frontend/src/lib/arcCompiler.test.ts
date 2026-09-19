import { describe, expect, it } from "vitest";
import {
    applyBriefToAnswers,
    compileArc,
    extractDatedSnippets,
    extractFacts,
    inferSpeechLevel,
    isAdmissionHistory,
    isRelativeVisitSentence,
    observationsAgitated,
    steadyStateLines,
    observationsNeedFixation,
    stripTherapyClauses,
    weekendDutyNote,
} from "./arcCompiler";
import {
    buildGenerateAnswers,
    intellectFromDiagnosis,
    shouldSkipWeekendDaily,
} from "./batchDiary";

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
      expect(b.forbidden.join(" ")).toMatch(/вед[её]т к двери/i);
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
    expect(firstMon?.weekendDutyNote).toBeNull();
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
    expect(arc).toMatch(/Понедельник после пропущенных сб\/вс ЭТОГО пакета/);
    expect(arc).toMatch(/Дополнительные сведения о заболевании/);
    expect(ans.additional_info).toBe("present");
    expect(String(ans.additional_info_detail)).toMatch(
      /за период выходных дней с 1-2\.08 под наблюдением дежурного мед персонала/,
    );
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

  it("does not dump an undated standing regimen into today's treatment plan", () => {
    for (const b of briefs) {
      expect(b.therapyToday).toBeNull();
    }
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
    expect(sat?.weekendDutyNote).toBeNull();
    expect(satAns.additional_info).toBe("none");
    expect(String(satAns.__arc_context__)).toMatch(/отдельный дневник за сегодня НЕ пишется/i);
    expect(sun?.weekendDutyNote).toBeNull();
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
    expect(arc).toMatch(/Критика: отсутствует/);
    expect(arc).toMatch(/адекватны ситуации/);
    expect(arc).toMatch(/Назначения.*см\. лист назначений/s);
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
    expect(mon?.forbidden.join(" ")).toMatch(/визиты родственников|мам|родительск/i);
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
    expect(exam?.therapyToday).toBeNull();
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
  it("formats the previous Saturday–Sunday on Monday", () => {
    expect(weekendDutyNote("2026-08-03", 10)).toBe(
      "за период выходных дней с 1-2.08 под наблюдением дежурного мед персонала.",
    );
    expect(weekendDutyNote("2026-08-01", 8)).toBeNull();
    expect(weekendDutyNote("2026-08-02", 9)).toBeNull();
  });

  it("formats a weekend that crosses months", () => {
    expect(weekendDutyNote("2026-11-02", 22)).toBe(
      "за период выходных дней с 31.10-1.11 под наблюдением дежурного мед персонала.",
    );
  });

  it("skips the first three hospital days even on Monday", () => {
    expect(weekendDutyNote("2026-08-03", 1)).toBeNull();
    expect(weekendDutyNote("2026-08-03", 2)).toBeNull();
    expect(weekendDutyNote("2026-08-03", 3)).toBeNull();
    expect(weekendDutyNote("2026-08-03", 4)).not.toBeNull();
  });

  it("skips non-Mondays", () => {
    expect(weekendDutyNote("2026-08-04", 11)).toBeNull();
    expect(weekendDutyNote("2026-08-07", 14)).toBeNull();
  });

  it("does not instruct the duty formula on admission-weekend days", () => {
    const days = [
      { isoDate: "2026-08-01", dayNumber: 1, documentType: "daily" as const },
      { isoDate: "2026-08-02", dayNumber: 2, documentType: "daily" as const },
      { isoDate: "2026-08-08", dayNumber: 8, documentType: "daily" as const },
      { isoDate: "2026-08-10", dayNumber: 10, documentType: "daily" as const },
    ];
    const briefs = compileArc({
      days,
      directorContext: "Состояние стабильно.",
      batchAnswers: { overall_dynamics: "stable" },
      estimatedDischargeDate: "",
    });
    expect(briefs[0].weekendDutyNote).toBeNull();
    expect(briefs[1].weekendDutyNote).toBeNull();
    expect(briefs[2].weekendDutyNote).toBeNull();
    expect(briefs[3].weekendDutyNote).toMatch(/с 8-9\.08/);
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
    expect(early.additional_info).toBe("none");
    expect(String(early.__arc_context__)).toMatch(/первых трёх дней/);
    expect(String(early.__arc_context__)).not.toMatch(/напиши РОВНО/);
  });
});

describe("stripTherapyClauses", () => {
  it("keeps the behavioral episode and drops the injection", () => {
    const src =
      "07.08. Р-р. Рисперидон 1% 0,75 мг/сут. Днем после завтра стал постоянно стремиться к движению, залезал на кровати, прыгал с них. В связи с этим с седативной целью была сделана инъекция р-ра. Хлорпромазина 2,5%-1 мл в/м.";
    const out = stripTherapyClauses(src);
    expect(out).toMatch(/залезал на кровати/);
    expect(out).not.toMatch(/инъекц/i);
    expect(out).not.toMatch(/мг\/сут/);
  });
});

describe("admission history vs in-ward day", () => {
  const narrative =
    "До госпитализации проживал в интернате. Сегодня мать забрала ребенка из интерната, планирует перевести его на пребывание в ДДИ 5 дней в неделю. В отделении был выдан пакет документов с направлением на госпитализацию в стационар. Двигательно расторможен, вербальной коррекции не поддавался, в связи с чем дежурным врачом была назначена инъекция раствора. Одевается, гигиенические мероприятия выполняет с помощью персонала. 07.08 выполнена инъекция р-ра хлорпромазина 2,5%-1 мл в/м.";

  it("recognizes internat, DDI and admission referral as history", () => {
    expect(isAdmissionHistory("мать забрала ребенка из интерната")).toBe(true);
    expect(isAdmissionHistory("направление на госпитализацию в стационар")).toBe(true);
    expect(isAdmissionHistory("пребывание в ДДИ 5 дней в неделю")).toBe(true);
    expect(isAdmissionHistory("в приёмном покое был неусидчив")).toBe(true);
    expect(isAdmissionHistory("доставлен сантранспортом в приемное отделение")).toBe(true);
    expect(isAdmissionHistory("после выписки рекомендованное лечение принимал неделю")).toBe(true);
    expect(isAdmissionHistory("вербальной коррекции не поддавался")).toBe(false);
    expect(isAdmissionHistory("в палате ходил, залезал на кровати")).toBe(false);
  });

  it("does not put internat pickup or referral into daily observations", () => {
    const days = packDays();
    const briefs = compileArc({
      days,
      directorContext: narrative,
      batchAnswers: {
        overall_dynamics: "wavy",
        diagnosis: "F72.14 Умственная отсталость тяжелая",
      },
      estimatedDischargeDate: "",
    });
    const dailyBlob = briefs
      .filter((b) => b.role !== "exam")
      .map((b) => b.observations.join(" "))
      .join("\n");
    expect(dailyBlob).not.toMatch(/интернат/i);
    expect(dailyBlob).not.toMatch(/\bдд[ие]\b/i);
    expect(dailyBlob).not.toMatch(/направлен\w*\s+на\s+госпитализац/i);
    expect(dailyBlob).not.toMatch(/пакет документов/i);
    expect(dailyBlob).not.toMatch(/ланирует/);
    expect(dailyBlob).not.toMatch(/инъекц/i);
    expect(dailyBlob).toMatch(/вербальн/i);
    const exam = briefs.find((b) => b.role === "exam");
    expect(exam?.historyNotes).toEqual([]);
    const examAns = buildGenerateAnswers(
      { overall_dynamics: "wavy", diagnosis: "F72.14 Умственная отсталость тяжелая" },
      exam!.dayNumber,
      days.length,
      exam!.isoDate,
      narrative,
      "",
      "exam_10d",
      exam,
    );
    const examArc = String(examAns.__arc_context__);
    expect(examArc).not.toMatch(/Фон\/анамнез поступления/);
    expect(examArc).not.toMatch(/проживал в интернате/i);
    expect(examArc).toMatch(/без дополнений/);
    expect(examAns.anamnesis_life).toBe("no_additions");
    const injectionDay = briefs.find((b) => b.isoDate === "2026-08-07");
    expect(injectionDay?.therapyToday).toMatch(/хлорпромазин|инъекц/i);
    const quiet = briefs.find((b) => b.isoDate === "2026-07-28");
    expect(quiet?.therapyToday).toBeNull();
    const ans = buildGenerateAnswers(
      { overall_dynamics: "wavy", diagnosis: "F72.14" },
      quiet!.dayNumber,
      days.length,
      quiet!.isoDate,
      narrative,
      "",
      "daily",
      quiet,
    );
    const arc = String(ans.__arc_context__);
    expect(arc).toMatch(/физическое удержание/);
    expect(arc).toMatch(/мягк[а-яё]* фиксац/);
    expect(arc).toMatch(/ЗАПРЕЩЕНО/);
    expect(arc).toMatch(/не выдумывай/);
    expect(arc).not.toMatch(/сегодня есть возбуждение/);
  });
});

describe("observationsNeedFixation", () => {
  it("flags agitation separately from hard verbal correction", () => {
    expect(
      observationsAgitated(["кричал, замахивался, вербальной коррекции не поддавался"]),
    ).toBe(true);
    expect(
      observationsAgitated([
        "вербальной коррекции поддаётся с трудом, на замечания реагирует непродолжительно",
      ]),
    ).toBe(false);
    expect(observationsAgitated(["в игровой смотрел телевизор"])).toBe(false);
  });

  it("requires the doctor to mention fixation — agitation alone is not enough", () => {
    expect(
      observationsNeedFixation(["кричал, замахивался, вербальной коррекции не поддавался"]),
    ).toBe(false);
    expect(
      observationsNeedFixation(["кричал, применена мягкая фиксация на 15 минут"]),
    ).toBe(true);
  });
});

describe("shouldSkipWeekendDaily", () => {
  it("skips Saturday/Sunday after hospital day 3", () => {
    expect(
      shouldSkipWeekendDaily({ isoDate: "2026-07-25", dayNumber: 23, documentType: "daily" }),
    ).toBe(true);
    expect(
      shouldSkipWeekendDaily({ isoDate: "2026-07-26", dayNumber: 24, documentType: "daily" }),
    ).toBe(true);
  });

  it("keeps the first three hospital days and weekdays and 10-day exams", () => {
    expect(
      shouldSkipWeekendDaily({ isoDate: "2026-08-01", dayNumber: 2, documentType: "daily" }),
    ).toBe(false);
    expect(
      shouldSkipWeekendDaily({ isoDate: "2026-07-27", dayNumber: 8, documentType: "daily" }),
    ).toBe(false);
    expect(
      shouldSkipWeekendDaily({ isoDate: "2026-08-01", dayNumber: 20, documentType: "exam_10d" }),
    ).toBe(false);
  });
});

describe("intellectFromDiagnosis", () => {
  it("maps F71/F72 to moderate/severe ID, not age norm", () => {
    expect(intellectFromDiagnosis("F71.18 Умственная отсталость умеренная")).toBe(
      "moderate_id",
    );
    expect(intellectFromDiagnosis("F72.14 Умственная отсталость тяжелая")).toBe(
      "severe_id",
    );
    expect(intellectFromDiagnosis("F92.8")).toBe("age_norm");
  });
});

// Синтетический кейс по мотивам прода (депрессия, выписка по требованию):
// пакет 11–16.09, поступление 09.09.
const DEPRESSIVE_NARRATIVE =
  "При поступлении был напряжен, тревожен, плаксив. Адаптировался постепенно, был тих, малозаметен, начал выборочно общаться со сверстниками. С мед. персоналом вежлив. На фоне приема таб. Флуоксетина 20 мг/сут тревога уменьшилась. 16.09.2026 в 12:00 в отделение пришла мать пациента. Мать и пациент выразили желание выписаться по категорическому требованию. С матерью и мальчиком проведена беседа о нежелательности преждевременной выписки. Решения они не поменяли, настаивают на выписке.\n" +
  "Состояние в целом стабильное, опасных тенденций не обнаруживает. Оснований для госпитализации в недобровольном порядке согласно ст. 29 нет. Может быть выписан по требованию.";

function depressiveDays() {
  return [11, 12, 13, 14, 15, 16].map((d, i) => ({
    isoDate: `2026-09-${d}`,
    dayNumber: i + 3,
    documentType: "daily" as const,
  }));
}

function depressiveBriefs() {
  return compileArc({
    days: depressiveDays(),
    directorContext: DEPRESSIVE_NARRATIVE,
    batchAnswers: {
      diagnosis: "F32.1 Депрессивный эпизод средней степени",
      leading_syndrome: "depressive",
      patient_sex: "male",
    },
    estimatedDischargeDate: "2026-09-16",
  });
}

describe("narrative parsing (prod regression 16.09)", () => {
  it("does not split sentences on мед. / таб. / ст. abbreviations", () => {
    const all = depressiveBriefs().flatMap((b) => [...b.observations, ...b.forbidden]);
    expect(all.some((o) => /^С мед$|^персоналом/.test(o.trim()))).toBe(false);
    expect(all.some((o) => /^\S{1,3}$/.test(o.trim()))).toBe(false);
  });

  it("keeps the whole dated episode on its day, including the discharge demand", () => {
    const last = depressiveBriefs().at(-1)!;
    const obs = last.observations.join(" ");
    expect(obs).toMatch(/пришла мать/);
    expect(obs).toMatch(/категорическому требованию/);
    expect(obs).toMatch(/нежелательности преждевременной выписки/);
    expect(obs).toMatch(/настаивают на выписке/);
  });

  it("puts the discharge conclusion on the last day only", () => {
    const briefs = depressiveBriefs();
    expect(briefs.at(-1)!.observations.join(" ")).toMatch(/недобровольном|выписан по требованию/);
    for (const b of briefs.slice(0, -1)) {
      expect(b.observations.join(" ")).not.toMatch(/требовани|недобровольн|настаивают на выписке/);
    }
  });

  it("recognizes «Мать» as a relative visit", () => {
    expect(isRelativeVisitSentence("Пришла мать пациента.")).toBe(true);
  });

  it("does not make a quiet depressive child restless or irritable", () => {
    for (const b of depressiveBriefs()) {
      expect(b.behavior).not.toBe("restless");
      expect(b.moodDetail).not.toContain("irritability");
    }
  });

  it("keeps «НЕ повторяй» short: labels, not other days' text or therapy", () => {
    for (const b of depressiveBriefs()) {
      expect(b.forbidden.length).toBeLessThanOrEqual(8);
      for (const f of b.forbidden) {
        expect(f.length).toBeLessThanOrEqual(90);
        expect(f).not.toMatch(/мг\/сут|Флуоксетин/i);
      }
    }
  });

  it("keeps a dated sentence that itself demands discharge on its own day", () => {
    const briefs = compileArc({
      days: depressiveDays(),
      directorContext: "Был тих, малозаметен. 15.09.2026 мать потребовала выписку по требованию.",
      batchAnswers: { leading_syndrome: "depressive" },
      estimatedDischargeDate: "2026-09-16",
    });
    const day15 = briefs.find((b) => b.isoDate === "2026-09-15")!;
    expect(day15.observations.join(" ")).toMatch(/потребовала выписку/);
  });

  it("does not pull background prose into a dated episode", () => {
    const briefs = compileArc({
      days: depressiveDays(),
      directorContext:
        "15.09.2026 пришла мать пациента. Фон настроения был снижен. В беседе с врачом вступал в контакт постепенно.",
      batchAnswers: { leading_syndrome: "depressive" },
      estimatedDischargeDate: "2026-09-16",
    });
    const day15 = briefs.find((b) => b.isoDate === "2026-09-15")!;
    const episode = day15.observations.find((o) => /пришла мать/.test(o))!;
    expect(episode).not.toMatch(/Фон настроения|В беседе с врачом/);
  });

  it("does not write discharge into a packet that ends before the planned discharge", () => {
    const briefs = compileArc({
      days: depressiveDays(),
      directorContext: "Был тих. Может быть выписан по требованию под наблюдение психиатра.",
      batchAnswers: { leading_syndrome: "depressive" },
      estimatedDischargeDate: "2026-09-30",
    });
    for (const b of briefs) {
      expect(b.observations.join(" ")).not.toMatch(/выписан по требованию/);
    }
  });

  it("reads «агрессию не проявлял» as a negation, not as agitation", () => {
    const briefs = compileArc({
      days: depressiveDays(),
      directorContext:
        "При поступлении был напряжен, тревожен, плаксив. Был тих, малозаметен. В общении со сверстниками в конфликтные ситуации не вступал, агрессию не проявлял.",
      batchAnswers: { leading_syndrome: "depressive" },
      estimatedDischargeDate: "2026-09-16",
    });
    for (const b of briefs) {
      expect(b.behavior).not.toBe("restless");
      expect(b.moodDetail).not.toContain("irritability");
    }
    expect(observationsAgitated(["агрессию не проявлял, возбуждений не было"])).toBe(false);
  });
});

// Синтетический нарратив в стиле «92.0»: статус при поступлении + анамнез
// суицидального поведения в начале текста.
const ADMISSION_BLOCK_NARRATIVE =
  "Статус при поступлении: Сознание ясное. Внешний вид неопрятный, волосы длинные, лицо закрыто волосами. Собственная речь развернутыми фразами, тихим голосом. В беседе причины госпитализации объяснил: «суицидальные мысли». Со слов, такие мысли появились около 2 лет назад после смерти отца. Со слов было 2 попытки суицида, наносил порезы, «купил лезвия, порезал руку». Критика к своему состоянию снижена. " +
  "Первое время был понур, тревожен. Постепенно адаптировался к режиму отделения, начал понемногу контактировать с другими ребятами. В течение дня занимал себя в игровой комнате, рисовал, читал.";

function admissionBlockBriefs() {
  const days = [5, 7, 8, 9, 10, 11, 14, 15, 16, 17, 18].map((d, i) => ({
    isoDate: `2026-09-${String(d).padStart(2, "0")}`,
    dayNumber: d - 2,
    documentType: "daily" as const,
  }));
  return compileArc({
    days,
    directorContext: ADMISSION_BLOCK_NARRATIVE,
    batchAnswers: { leading_syndrome: "anxious", diagnosis: "F92.0" },
    estimatedDischargeDate: "2026-09-18",
  });
}

describe("admission status and suicidal history (prod 92.0)", () => {
  it("never turns suicide attempts / self-harm history into a day event", () => {
    for (const b of admissionBlockBriefs()) {
      expect(b.observations.join(" ")).not.toMatch(
        /суицид|лезви|порез|убить себя|лет назад|смерти отца/i,
      );
    }
  });

  it("keeps the admission status on the first days only", () => {
    const briefs = admissionBlockBriefs();
    expect(briefs[0].observations.join(" ")).toMatch(/неопрятный|развернутыми фразами|понур/);
    for (const b of briefs.filter((x) => x.phase !== "admission")) {
      expect(b.observations.join(" ")).not.toMatch(/лицо закрыто волосами|неопрятный/);
    }
    expect(briefs.flatMap((b) => b.observations).join(" ")).not.toMatch(/статус при поступлении/i);
  });

  it("does not make «тихим голосом» a standalone observation", () => {
    for (const b of admissionBlockBriefs()) {
      for (const o of b.observations) {
        expect(o.trim()).not.toMatch(/^(?:тихим голосом|волосы длинные|лицо закрыто волосами)$/);
      }
    }
  });

  it("treats reported suicidal thoughts as history but keeps their denial", () => {
    expect(isAdmissionHistory("Сообщал о снижении настроения, суицидальных мыслях.")).toBe(true);
    expect(
      isAdmissionHistory("Суицидные и парасуицидные тенденции категорически отрицал."),
    ).toBe(false);
    const briefs = compileArc({
      days: depressiveDays(),
      directorContext:
        "Был тих. В беседе сообщал о длительном ухудшении состояния, суицидальных мыслях. Суицидные тенденции категорически отрицал. В течение дня читал книги в игровой комнате.",
      batchAnswers: { leading_syndrome: "depressive" },
      estimatedDischargeDate: "2026-09-16",
    });
    for (const b of briefs) {
      expect(b.observations.join(" ")).not.toMatch(/о длительном ухудшении|суицидальных мыслях/);
    }
    expect(briefs.flatMap((b) => b.observations).join(" ")).toMatch(/отрицал/);
  });

  it("does not drag a mid-stay «на момент осмотра» sentence to the discharge day", () => {
    const briefs = compileArc({
      days: depressiveDays(),
      directorContext:
        "Был тих, малозаметен. Психопродуктивной симптоматики на момент осмотра не выявлено. В течение дня читал книги в игровой комнате.",
      batchAnswers: { leading_syndrome: "depressive" },
      estimatedDischargeDate: "2026-09-16",
    });
    const early = briefs.slice(0, -1).flatMap((b) => b.observations).join(" ");
    expect(early).toMatch(/на момент осмотра/);
  });

  it("locks orientation and criticism so they do not drift between days", () => {
    const days = [11, 12, 13, 14, 15, 16].map((d, i) => ({
      isoDate: `2026-09-${d}`,
      dayNumber: i + 3,
      documentType: "daily" as const,
    }));
    const briefs = compileArc({
      days,
      directorContext: "Был тих, малозаметен. В течение дня читал книги в игровой комнате.",
      batchAnswers: {
        leading_syndrome: "depressive",
        diagnosis: "F32.1 депрессивный эпизод",
        final_state: "Критика к своему состоянию и заболеванию формируется.",
      },
      estimatedDischargeDate: "2026-09-16",
    });
    for (const b of briefs) {
      const arc = String(
        applyBriefToAnswers({}, b, { diagnosis: "F32.1 депрессивный эпизод",
          final_state: "Критика к своему состоянию и заболеванию формируется." },
          "2026-09-16").__arc_context__,
      );
      expect(arc).toMatch(/Ориентировка — константа всех дней/);
      expect(arc).toMatch(/Критика — обязательный элемент статуса/);
      expect(arc).toMatch(/Не чередуй/);
    }
  });

  it("keeps the intellectual-disability orientation wording free", () => {
    const lines = steadyStateLines("F71.18 умственная отсталость умеренная", "", "field");
    expect(lines.join(" ")).not.toMatch(/Ориентировка — константа/);
    expect(lines.join(" ")).toMatch(/Критика/);
  });

  it("does not repeat the same occupation on consecutive days", () => {
    const days = [11, 12, 13, 14, 15, 16].map((d, i) => ({
      isoDate: `2026-09-${d}`,
      dayNumber: i + 4,
      documentType: "daily" as const,
    }));
    const briefs = compileArc({
      days,
      directorContext:
        "Был тих, малозаметен. В течение дня занимал себя в игровой комнате, рисовал, читал. Постепенно стал спокойнее.",
      batchAnswers: { leading_syndrome: "depressive" },
      estimatedDischargeDate: "2026-09-16",
    });
    for (let i = 1; i < briefs.length; i++) {
      const window = briefs.slice(Math.max(0, i - 3), i).flatMap((b) => b.observations);
      for (const o of briefs[i].observations.filter((x) => /игрово/.test(x))) {
        expect(window).not.toContain(o);
      }
    }
  });

  it("keeps the doctor's intellect wording instead of rounding it", () => {
    const lines = steadyStateLines(
      "F92.0 расстройство поведения",
      "Интеллектуально-мнестически на уровне низкой возрастной нормы. Критика формируется.",
      "residual",
    );
    expect(lines.join(" ")).toMatch(/низкой возрастной нормы/);
  });
});

