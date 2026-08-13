import { describe, expect, it } from "vitest";
import { omitEmptyDiarySections, parseDiaryMarkup } from "./diaryMarkup";

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

  it("leaves unmatched asterisks as text", () => {
    const runs = kinds("температура 36*7");
    expect(runs.every(([k]) => k !== "bold")).toBe(true);
    expect(runs.some(([, t]) => t.includes("36*7"))).toBe(true);
  });
});

describe("omitEmptyDiarySections", () => {
  it("omits empty filler sections and orphan «Без изменений»", () => {
    const text = omitEmptyDiarySections(
      [
        "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ",
        "Жалобы: Не предъявляет (ввиду особенностей речевого развития).",
        "Анамнез заболевания (дополнения к анамнезу): Без дополнений.",
        "Психический статус: Сознание ясное.",
        "План обследования (дополнения к плану):",
        "Без изменений.",
        "Без изменений",
      ].join("\n"),
    );
    expect(text).toContain("Психический статус:");
    expect(text).toContain("Сознание ясное.");
    expect(text.toLowerCase()).not.toContain("жалобы");
    expect(text.toLowerCase()).not.toContain("без дополнений");
    expect(text.toLowerCase()).not.toContain("без изменений");
    expect(text).not.toContain("План обследования");
    expect(text).toContain("ОСМОТР ЛЕЧАЩИМ ВРАЧОМ");
  });
});
