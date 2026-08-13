import { describe, expect, it } from "vitest";
import {
  applyDiaryStamp,
  buildExportSubstitutions,
  composeDailyDoctorLine,
  composeHeadSignature,
  diaryDateFromTitle,
  formatExportDate,
  signatureFields,
} from "./exportSubstitutions";
import { fixObviousTypos } from "./typoFixes";

describe("diary date stamp", () => {
  it("reads the examination day from a batch title", () => {
    expect(diaryDateFromTitle("День 8 · 27.07.2026 · Ежедневный осмотр")).toBe(
      "27.07.2026",
    );
  });

  it("formats ISO day as official MIS date", () => {
    expect(formatExportDate("2026-07-27")).toBe("«27» июля 2026 г.");
  });

  it("always stamps 10 час. 00 мин. and prefers title date over created_at", () => {
    const subs = buildExportSubstitutions({
      title: "День 11 · 30.07.2026 · Ежедневный осмотр",
      createdAt: "2026-08-13T08:00:39.15752Z",
      doctorName: "Врач Т.",
    });
    expect(subs["[ДАТА]"]).toBe("«30» июля 2026 г.");
    expect(subs["[ВРЕМЯ]"]).toBe("10 час. 00 мин.");
    expect(subs["[ФИО_ВРАЧА]"]).toBe("Врач Т.");
  });

  it("composes doctor and head signatures from the profile", () => {
    expect(
      signatureFields({
        full_name: "Иванов Иван Иванович",
        position: "Лечащий врач, врач-психиатр детский, психиатрия",
        head_full_name: "Петрова Анна Сергеевна",
      }),
    ).toEqual({
      doctorName: "Иванов Иван Иванович",
      doctorPosition: "врач-психиатр детский, психиатрия",
      headName: "Петрова Анна Сергеевна",
    });
    const subs = buildExportSubstitutions({
      doctor: {
        full_name: "Иванов Иван Иванович",
        position: "врач-психиатр детский",
        head_full_name: "Петрова А.С.",
        head_position: "Врач-психиатр детский",
        head_institution:
          "ОПО№1, Общепсихиатрическое отделение для обслуживания детского населения №2",
      },
    });
    expect(subs["[ФИО_ВРАЧА]"]).toBe("Иванов Иван Иванович");
    expect(subs["[ДОЛЖНОСТЬ_ВРАЧА]"]).toBe("врач-психиатр детский");
    expect(subs["[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]"]).toBe(
      "Петрова А.С. Врач-психиатр детский (ОПО№1). Общепсихиатрическое отделение для обслуживания детского населения №2",
    );
  });

  it("fixes obvious clinical typos in the preview stamp", () => {
    expect(fixObviousTypos("F71.18 Сидрос психмоторной расторможенности")).toBe(
      "F71.18 Синдром психомоторной расторможенности",
    );
    expect(fixObviousTypos("Жалобы: самостоятельно не предъявляет")).toBe(
      "Жалобы: не предъявляет",
    );
    const out = applyDiaryStamp(
      "Основное заболевание: F71.18 Сидрос психмоторной расторможенности",
      {},
    );
    expect(out).toContain("Синдром психомоторной");
    expect(out).not.toContain("Сидрос");
  });

  it("fills placeholders in preview text", () => {
    const out = applyDiaryStamp("[ДАТА] время: [ВРЕМЯ]", {
      title: "День 8 · 27.07.2026 · Ежедневный осмотр",
    });
    expect(out).toBe("«27» июля 2026 г. время: 10 час. 00 мин.");
  });

  it("rewrites a duplicated doctor caption before the head placeholder", () => {
    const out = applyDiaryStamp(
      "Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись\n" +
        "[ФИО_ВРАЧА]\n" +
        "Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись\n" +
        "[ФИО_ЗАВ_ОТДЕЛЕНИЕМ]",
      {},
    );
    expect(out).toContain(
      "Фамилия, имя, отчество (при наличии) заведующего отделением, подпись",
    );
    expect(out.split("врача, должность").length - 1).toBe(1);
  });

  it("rewrites old numeric headers", () => {
    const out = applyDiaryStamp("13.08.2026 время: 10:45", {});
    expect(out).toBe("«13» августа 2026 г. время: 10 час. 45 мин.");
  });

  it("orders daily signature as attending, position, then FIO", () => {
    expect(
      composeDailyDoctorLine({
        full_name: "Иванов Иван Иванович",
        position: "врач-психиатр детский",
      }),
    ).toBe("Лечащий врач, врач-психиатр детский, Иванов Иван Иванович");
    const out = applyDiaryStamp(
      "ОСМОТР ЛЕЧАЩИМ ВРАЧОМ\nЛечащий врач [ФИО_ВРАЧА]",
      {
        doctor: {
          full_name: "Иванов Иван Иванович",
          position: "врач-психиатр детский",
        },
      },
    );
    expect(out).toContain(
      "Лечащий врач, врач-психиатр детский, Иванов Иван Иванович",
    );
    expect(out).not.toContain("[ДОЛЖНОСТЬ_ВРАЧА]");
  });

  it("composes the department-head line with position and institution", () => {
    expect(
      composeHeadSignature({
        head_full_name: "Иванов Иван Иванович",
        head_position: "Врач-психиатр детский",
        head_institution:
          "ОПО№1, Общепсихиатрическое отделение для обслуживания детского населения №2",
      }),
    ).toBe(
      "Иванов Иван Иванович. Врач-психиатр детский (ОПО№1). Общепсихиатрическое отделение для обслуживания детского населения №2",
    );
  });
});
