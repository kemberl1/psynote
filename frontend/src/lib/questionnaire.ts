// Логика опросника (docs/06). Задел под Этап 7: тут собираются дефолты,
// вычисляется видимость условных вопросов и считается прогресс.
//
// ВАЖНО (граница Этапа 6/7): базовая реактивная видимость условных вопросов
// УЖЕ реализована (computeVisibleIds) — этого достаточно для аккуратного UX.
// Этап 7 расширит её более сложными правилами (мультизначные условия,
// вложенность, «свой вариант» как триггер и т.п.) и динамическими справочниками.
import type {
    AnswerValue,
    Answers,
    CustomAnswer,
    Question,
    QuestionnaireSchema,
} from "../api/types";

/** Маркер «своего варианта» (docs/06 §1.4). */
export const CUSTOM_VALUE = "__custom__";

/** Type guard для «своего варианта». */
export function isCustomAnswer(
  v: AnswerValue | undefined,
): v is CustomAnswer {
  return (
    typeof v === "object" &&
    v !== null &&
    !Array.isArray(v) &&
    (v as CustomAnswer).value === CUSTOM_VALUE
  );
}

/** Строит начальные ответы из default-значений схемы (docs/06 §1.5). */
export function buildDefaults(schema: QuestionnaireSchema): Answers {
  const answers: Answers = {};
  for (const q of schema.questions) {
    if (q.default !== undefined && q.default !== null) {
      answers[q.id] = q.default as AnswerValue;
    } else if (q.type === "multiselect") {
      answers[q.id] = [];
    }
  }
  return answers;
}

/**
 * Вычисляет множество видимых вопросов с учётом условной логики (docs/06 §3).
 * Вопрос виден, если он не является «дочерним» ни одного conditional, ЛИБО
 * родитель имеет соответствующее значение (if_value). Поддерживает каскад
 * (родитель сам должен быть видим, чтобы показать ребёнка).
 */
export function computeVisibleIds(
  schema: QuestionnaireSchema,
  answers: Answers,
): Set<string> {
  // 1. Собираем все «условно-показываемые» id и их триггеры.
  const triggeredBy = new Map<string, { parentId: string; ifValue: string }[]>();
  for (const q of schema.questions) {
    for (const cond of q.conditional ?? []) {
      for (const showId of cond.show) {
        const list = triggeredBy.get(showId) ?? [];
        list.push({ parentId: q.id, ifValue: cond.if_value });
        triggeredBy.set(showId, list);
      }
    }
  }

  const visible = new Set<string>();
  // Итеративно (до стабилизации) — для поддержки каскада «родитель→ребёнок».
  let changed = true;
  const isConditional = (id: string) => triggeredBy.has(id);

  // Изначально видимы все безусловные вопросы.
  for (const q of schema.questions) {
    if (!isConditional(q.id)) visible.add(q.id);
  }

  while (changed) {
    changed = false;
    for (const q of schema.questions) {
      if (!isConditional(q.id) || visible.has(q.id)) continue;
      const triggers = triggeredBy.get(q.id) ?? [];
      const shown = triggers.some(
        (t) => visible.has(t.parentId) && answerMatches(answers[t.parentId], t.ifValue),
      );
      if (shown) {
        visible.add(q.id);
        changed = true;
      }
    }
  }
  return visible;
}

/** Проверяет, удовлетворяет ли ответ значению-триггеру (скаляр или в массиве). */
function answerMatches(value: AnswerValue | undefined, ifValue: string): boolean {
  if (value === undefined || value === null) return false;
  if (Array.isArray(value)) return value.includes(ifValue);
  if (isCustomAnswer(value)) return value.value === ifValue;
  return String(value) === ifValue;
}

/** Считает прогресс: сколько обязательных видимых вопросов отвечено (docs/08 §5.1). */
export function computeProgress(
  schema: QuestionnaireSchema,
  answers: Answers,
  visible: Set<string>,
): { answered: number; total: number; missingRequired: string[] } {
  const required = schema.questions.filter(
    (q) => q.required && visible.has(q.id),
  );
  const missingRequired: string[] = [];
  let answered = 0;
  for (const q of required) {
    if (isAnswered(answers[q.id])) answered += 1;
    else missingRequired.push(q.id);
  }
  return { answered, total: required.length, missingRequired };
}

/** Считается ли вопрос отвеченным. */
export function isAnswered(value: AnswerValue | undefined): boolean {
  if (value === undefined || value === null) return false;
  if (Array.isArray(value)) return value.length > 0;
  if (isCustomAnswer(value)) return value.custom_text.trim().length > 0;
  if (typeof value === "string") return value.trim().length > 0;
  return true; // number | boolean
}

/**
 * Готовит answers к отправке: убирает значения скрытых вопросов (они не должны
 * влиять на генерацию) и пустые «свои варианты». Возвращает чистую карту.
 */
export function prepareAnswers(
  schema: QuestionnaireSchema,
  answers: Answers,
  visible: Set<string>,
): Answers {
  const out: Answers = {};
  for (const q of schema.questions) {
    if (!visible.has(q.id)) continue;
    const value = answers[q.id];
    if (!isAnswered(value)) continue;
    out[q.id] = value as AnswerValue;
  }
  return out;
}

/** Удобный селектор вопроса по id. */
export function questionById(
  schema: QuestionnaireSchema,
  id: string,
): Question | undefined {
  return schema.questions.find((q) => q.id === id);
}
