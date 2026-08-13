import { describe, expect, it } from "vitest";
import { omitEmptyDiarySections, normalizeDailySpacing, parseDiaryMarkup } from "./diaryMarkup";

function kinds(content: string) {
  return parseDiaryMarkup(content).map((r) => [r.kind, r.text] as const);
}

describe("parseDiaryMarkup", () => {
  it("renders **text** as bold", () => {
    const runs = kinds("до **Жалобы:** после");
    expect(runs).toContainEqual(["bold", "Жалобы:"]);
    expect(runs.some(([k, t]) => k === "text" && t.includes("до"))).toBe(true);
  });

  it("bolds template section labels without markdown", () => {
    const runs = kinds("Жалобы: Не предъявляет.\nПсихический статус: Спокоен.");
    expect(runs).toContainEqual(["bold", "Жалобы:"]);
    expect(runs).toContainEqual(["bold", "Психический статус:"]);
    expect(runs.some(([k, t]) => k === "text" && t.includes("Не предъявляет"))).toBe(
      true,
    );
  });

  it("strips ** around section labels and still bolds them", () => {
    const runs = kinds("**Жалобы:** Не предъявляет.");
    expect(runs).toContainEqual(["bold", "Жалобы:"]);
    expect(runs.every(([, t]) => !t.includes("**"))).toBe(true);
  });

  it("keeps placeholders highlighted", () => {
    const runs = kinds("Лечащий врач: [ФИО_ВРАЧА]");
    expect(runs).toContainEqual(["bold", "Лечащий врач:"]);
    expect(runs).toContainEqual(["placeholder", "[ФИО_ВРАЧА]"]);
  });

  it("bolds the daily exam header", () => {
    const runs = kinds("ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\n[ДАТА] время: [ВРЕМЯ]");
    expect(runs).toContainEqual(["bold", "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ"]);
    expect(runs).toContainEqual(["placeholder", "[ДАТА]"]);
  });

  it("does not bold values after a section colon", () => {
    const runs = kinds(
      "Анамнез заболевания (дополнения к анамнезу): **без дополнений**\n" +
        "Основное заболевание: F71.18 синдром",
    );
    expect(runs).toContainEqual([
      "bold",
      "Анамнез заболевания (дополнения к анамнезу):",
    ]);
    expect(runs).toContainEqual(["bold", "Основное заболевание:"]);
    expect(runs.some(([k, t]) => k === "bold" && t.includes("без дополнений"))).toBe(
      false,
    );
    expect(runs.some(([k, t]) => k === "text" && t.includes("без дополнений"))).toBe(
      true,
    );
    expect(runs.some(([k, t]) => k === "bold" && t.includes("F71.18"))).toBe(false);
  });

  it("bolds MIS diagnosis-adjacent labels", () => {
    const runs = kinds(
      "Дополнительные сведения о заболевании: нет\n" +
        "Обоснование диагноза (при наличии дополнительных сведений): не требуется\n" +
        "План лечения (дополнения к плану): без дополнений",
    );
    expect(runs).toContainEqual(["bold", "Дополнительные сведения о заболевании:"]);
    expect(runs).toContainEqual([
      "bold",
      "Обоснование диагноза (при наличии дополнительных сведений):",
    ]);
    expect(runs).toContainEqual(["bold", "План лечения (дополнения к плану):"]);
  });

  it("leaves unmatched asterisks as text", () => {
    const runs = kinds("температура 36*7");
    expect(runs.every(([k]) => k !== "bold")).toBe(true);
    expect(runs.some(([, t]) => t.includes("36*7"))).toBe(true);
  });
});

describe("omitEmptyDiarySections", () => {
  it("keeps MIS defaults and drops «Данных нет»", () => {
    const text = omitEmptyDiarySections(
      [
        "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ",
        "Жалобы: не предъявляет",
        "Анамнез заболевания (дополнения к анамнезу): Без дополнений.",
        "Психический статус: Сознание ясное.",
        "План обследования (дополнения к плану): без дополнений",
        "Жалобы: Данных нет",
      ].join("\n"),
    );
    expect(text).toContain("Психический статус:");
    expect(text).toContain("Сознание ясное.");
    expect(text.toLowerCase()).toContain("жалобы: не предъявляет");
    expect(text.toLowerCase()).toContain("без дополнений");
    expect(text).toContain("План обследования");
    expect(text).not.toContain("Данных нет");
    expect(text).toContain("ОСМОТР ЛЕЧАЩИМ ВРАЧОМ");
  });
});

describe("normalizeDailySpacing", () => {
  it("keeps one blank after anamnesis and neurological status only", () => {
    const text = normalizeDailySpacing(
      [
        "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ",
        "Жалобы: не предъявляет",
        "",
        "Анамнез заболевания (дополнения к анамнезу): без дополнений",
        "Анамнез жизни (дополнения к анамнезу): без дополнений",
        "Физикальное исследование, локальный статус (его изменение): Т 36,6",
        "Психический статус: Сознание ясное.",
        "Неврологический статус: без острой неврологической симптоматики",
        "Диагноз:",
      ].join("\n"),
    );
    expect(text).toContain(
      "Анамнез жизни (дополнения к анамнезу): без дополнений\n\nФизикальное",
    );
    expect(text).toContain(
      "Неврологический статус: без острой неврологической симптоматики\n\nДиагноз:",
    );
    expect(text).not.toContain("Жалобы: не предъявляет\n\nАнамнез заболевания");
  });

  it("does not change 10-day spacing", () => {
    const src = [
      "ОСМОТР",
      "лечащим врачом совместно с заведующим отделением",
      "Анамнез жизни (дополнения к анамнезу): без дополнений",
      "Неврологический статус (его изменение): без острой неврологической симптоматики",
      "Диагноз:",
    ].join("\n");
    expect(normalizeDailySpacing(src)).toBe(src);
  });
});
