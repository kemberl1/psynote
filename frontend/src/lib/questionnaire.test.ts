// Юнит-тесты логики опросника (Этап 7, docs/06 §3).
// Проверяем: дефолты, условную видимость + каскад, multiselect-триггеры,
// «свой вариант» как триггер, очистку скрытых ответов, прогресс/валидацию,
// подготовку payload и группировку по секциям.
//
// Запуск: cd frontend && npm test
import { describe, expect, it } from "vitest";
import type { Answers, QuestionnaireSchema } from "../api/types";
import {
    buildDefaults,
    clearHiddenAnswers,
    computeProgress,
    computeVisibleIds,
    CUSTOM_VALUE,
    groupVisibleQuestions,
    isAnswered,
    prepareAnswers,
} from "./questionnaire";

// Мини-схема, повторяющая ключевые ветвления docs/06: select→cascade,
// multiselect-триггер, boolean-триггер, группы.
const schema: QuestionnaireSchema = {
  document_type: "test",
  version: 1,
  questions: [
    {
      id: "mood",
      label: "Настроение",
      type: "select",
      required: true,
      allow_custom: true,
      default: "even",
      group: "Состояние",
      options: [
        { value: "even", label: "ровное" },
        { value: "lowered", label: "снижено" },
      ],
      conditional: [{ if_value: "lowered", show: ["mood_detail"] }],
    },
    {
      id: "mood_detail",
      label: "Детали",
      type: "multiselect",
      required: false,
      allow_custom: true,
      group: "Состояние",
      options: [
        { value: "anxiety", label: "тревога" },
        { value: "tearfulness", label: "плаксивость" },
      ],
    },
    {
      id: "events",
      label: "События",
      type: "multiselect",
      required: false,
      allow_custom: true,
      group: "События",
      options: [
        { value: "consultation", label: "консультация" },
        { value: "exam", label: "обследование" },
      ],
      conditional: [
        { if_value: "consultation", show: ["events_detail"] },
        { if_value: "exam", show: ["events_detail"] },
      ],
    },
    {
      id: "events_detail",
      label: "Детали событий",
      type: "text",
      required: false,
      allow_custom: true,
      group: "События",
    },
    {
      id: "discharge",
      label: "Выписка?",
      type: "boolean",
      required: false,
      allow_custom: false,
      group: "Эпикриз",
      conditional: [{ if_value: "true", show: ["discharge_detail"] }],
    },
    {
      id: "discharge_detail",
      label: "Заключение",
      type: "text",
      required: false,
      allow_custom: true,
      group: "Эпикриз",
    },
  ],
};

describe("buildDefaults", () => {
  it("берёт default и инициализирует multiselect пустым массивом", () => {
    const d = buildDefaults(schema);
    expect(d.mood).toBe("even");
    expect(d.mood_detail).toEqual([]);
    expect(d.events).toEqual([]);
    expect(d.discharge).toBeUndefined();
  });
});

describe("computeVisibleIds", () => {
  it("скрывает условный вопрос при дефолте и показывает при триггере", () => {
    const base = buildDefaults(schema);
    expect(computeVisibleIds(schema, base).has("mood_detail")).toBe(false);

    const triggered = { ...base, mood: "lowered" };
    expect(computeVisibleIds(schema, triggered).has("mood_detail")).toBe(true);
  });

  it("multiselect-триггер: вопрос виден, если значение выбрано", () => {
    const a: Answers = { ...buildDefaults(schema), events: ["consultation"] };
    expect(computeVisibleIds(schema, a).has("events_detail")).toBe(true);

    const b: Answers = { ...buildDefaults(schema), events: [] };
    expect(computeVisibleIds(schema, b).has("events_detail")).toBe(false);
  });

  it("boolean-триггер true показывает дочерний вопрос", () => {
    const a: Answers = { ...buildDefaults(schema), discharge: true };
    expect(computeVisibleIds(schema, a).has("discharge_detail")).toBe(true);
    const b: Answers = { ...buildDefaults(schema), discharge: false };
    expect(computeVisibleIds(schema, b).has("discharge_detail")).toBe(false);
  });

  it("«свой вариант» как значение select не ломает видимость", () => {
    const a: Answers = {
      ...buildDefaults(schema),
      mood: { value: CUSTOM_VALUE, custom_text: "своё" },
    };
    // mood_detail привязан к if_value "lowered", кастом != lowered → скрыт.
    expect(computeVisibleIds(schema, a).has("mood_detail")).toBe(false);
  });
});

describe("clearHiddenAnswers", () => {
  it("очищает ответ вопроса, ставшего невидимым", () => {
    let answers: Answers = {
      ...buildDefaults(schema),
      mood: "lowered",
      mood_detail: ["anxiety"],
    };
    // Возвращаем к even → mood_detail должен очиститься.
    answers = { ...answers, mood: "even" };
    const visible = computeVisibleIds(schema, answers);
    const cleaned = clearHiddenAnswers(schema, answers, visible);
    expect(cleaned.mood_detail).toEqual([]); // multiselect → []
  });

  it("удаляет ключ скрытого text-вопроса", () => {
    let answers: Answers = {
      ...buildDefaults(schema),
      events: ["consultation"],
      events_detail: "был психолог",
    };
    answers = { ...answers, events: [] };
    const visible = computeVisibleIds(schema, answers);
    const cleaned = clearHiddenAnswers(schema, answers, visible);
    expect(cleaned.events_detail).toBeUndefined();
  });

  it("возвращает ту же ссылку, если чистить нечего", () => {
    const answers = buildDefaults(schema);
    const visible = computeVisibleIds(schema, answers);
    expect(clearHiddenAnswers(schema, answers, visible)).toBe(answers);
  });
});

describe("computeProgress", () => {
  it("считает обязательные видимые и недостающие", () => {
    const answers: Answers = { mood_detail: [], events: [] }; // mood не отвечен
    const visible = computeVisibleIds(schema, answers);
    const p = computeProgress(schema, answers, visible);
    expect(p.total).toBe(1); // только mood обязателен
    expect(p.answered).toBe(0);
    expect(p.missingRequired).toContain("mood");
  });
});

describe("isAnswered", () => {
  it("multiselect с пустым кастомом не считается отвеченным", () => {
    expect(isAnswered([{ value: CUSTOM_VALUE, custom_text: "  " }])).toBe(false);
    expect(isAnswered([{ value: CUSTOM_VALUE, custom_text: "x" }])).toBe(true);
    expect(isAnswered(["anxiety"])).toBe(true);
    expect(isAnswered([])).toBe(false);
  });
});

describe("prepareAnswers", () => {
  it("исключает скрытые вопросы и пустые кастомы multiselect", () => {
    const answers: Answers = {
      mood: "even",
      mood_detail: ["anxiety"], // скрыт (mood=even) → должен уйти
      events: ["exam", { value: CUSTOM_VALUE, custom_text: "  " }],
      events_detail: "результаты",
    };
    const visible = computeVisibleIds(schema, answers);
    const payload = prepareAnswers(schema, answers, visible);
    expect(payload.mood).toBe("even");
    expect(payload.mood_detail).toBeUndefined();
    // events: пустой кастом отброшен, остался код.
    expect(payload.events).toEqual(["exam"]);
    expect(payload.events_detail).toBe("результаты");
  });
});

describe("groupVisibleQuestions", () => {
  it("группирует по секциям с порядком родитель→дети", () => {
    const answers: Answers = {
      ...buildDefaults(schema),
      mood: "lowered",
      events: ["consultation"],
    };
    const visible = computeVisibleIds(schema, answers);
    const groups = groupVisibleQuestions(schema, visible);
    const names = groups.map((g) => g.name);
    // discharge (boolean) безусловен → группа «Эпикриз» тоже присутствует.
    expect(names).toEqual(["Состояние", "События", "Эпикриз"]);
    // mood_detail сразу после mood в группе «Состояние».
    const state = groups[0].questions.map((q) => q.id);
    expect(state).toEqual(["mood", "mood_detail"]);
    const events = groups[1].questions.map((q) => q.id);
    expect(events).toEqual(["events", "events_detail"]);
  });
});
