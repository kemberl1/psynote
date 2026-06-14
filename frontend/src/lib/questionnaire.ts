// Логика опросника (docs/06, Этап 7). Здесь собираются дефолты, вычисляется
// видимость условных вопросов (с каскадом), очищаются ответы скрытых вопросов,
// считается прогресс и готовится payload к отправке. Также — группировка
// вопросов по секциям (docs/08 §5.1) и хелперы «своего варианта», в т.ч. внутри
// multiselect.
//
// Полная условная логика (docs/06 §3):
//   • show_if по конкретному значению (if_value) — для select/boolean;
//   • для multiselect триггер срабатывает, если значение ВХОДИТ в массив выбора;
//   • «свой вариант» (__custom__) тоже может быть триггером (по value === if_value);
//   • каскад: дочерний вопрос виден только если виден его родитель;
//   • скрытие вопроса очищает его ответ (clearHiddenAnswers), чтобы скрытые
//     значения не влияли на генерацию и не «всплывали» при повторном показе.
import type {
  AnswerValue,
  Answers,
  CustomAnswer,
  MultiAnswerItem,
  Question,
  QuestionnaireSchema,
} from "../api/types";

/** Маркер «своего варианта» (docs/06 §1.4). */
export const CUSTOM_VALUE = "__custom__";

/** Type guard для «своего варианта». */
export function isCustomAnswer(
  v: AnswerValue | MultiAnswerItem | undefined,
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
 * хотя бы один родитель видим и имеет соответствующее значение (if_value).
 * Поддерживает каскад (родитель сам должен быть видим, чтобы показать ребёнка).
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
  const isConditional = (id: string) => triggeredBy.has(id);

  // Изначально видимы все безусловные вопросы.
  for (const q of schema.questions) {
    if (!isConditional(q.id)) visible.add(q.id);
  }

  // Итеративно (до стабилизации) — для поддержки каскада «родитель→ребёнок».
  let changed = true;
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

/**
 * Проверяет, удовлетворяет ли ответ значению-триггеру.
 *   • массив (multiselect): if_value входит в выбранные коды;
 *   • «свой вариант»: совпадение по value (обычно if_value === "__custom__");
 *   • boolean: сравнение со строками "true"/"false";
 *   • скаляр: строковое сравнение.
 */
function answerMatches(value: AnswerValue | undefined, ifValue: string): boolean {
  if (value === undefined || value === null) return false;
  if (Array.isArray(value)) {
    return value.some((item) =>
      isCustomAnswer(item) ? item.value === ifValue : item === ifValue,
    );
  }
  if (isCustomAnswer(value)) return value.value === ifValue;
  if (typeof value === "boolean") return String(value) === ifValue;
  return String(value) === ifValue;
}

/**
 * Очищает ответы вопросов, которые стали невидимыми (docs/06 §3 — скрытие
 * очищает ответ). Возвращает НОВУЮ карту, если что-то изменилось, иначе
 * исходную ссылку (для стабильности рендера). Для multiselect ставит [],
 * для остального — удаляет ключ.
 */
export function clearHiddenAnswers(
  schema: QuestionnaireSchema,
  answers: Answers,
  visible: Set<string>,
): Answers {
  let mutated = false;
  const next: Answers = { ...answers };
  for (const q of schema.questions) {
    if (visible.has(q.id)) continue;
    if (!(q.id in next)) continue;
    if (q.type === "multiselect") {
      const cur = next[q.id];
      if (Array.isArray(cur) && cur.length === 0) continue;
      next[q.id] = [];
    } else {
      delete next[q.id];
    }
    mutated = true;
  }
  return mutated ? next : answers;
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
  if (Array.isArray(value)) {
    return value.some((item) =>
      isCustomAnswer(item) ? item.custom_text.trim().length > 0 : true,
    );
  }
  if (isCustomAnswer(value)) return value.custom_text.trim().length > 0;
  if (typeof value === "string") return value.trim().length > 0;
  return true; // number | boolean
}

/**
 * Готовит answers к отправке: оставляет только видимые отвеченные вопросы,
 * чистит multiselect от пустых «своих вариантов». Скрытые вопросы исключаются
 * (их значения не должны влиять на генерацию).
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
    if (Array.isArray(value)) {
      // Отбрасываем пустые кастом-элементы multiselect.
      const cleaned = value.filter((item) =>
        isCustomAnswer(item) ? item.custom_text.trim().length > 0 : true,
      );
      if (cleaned.length === 0) continue;
      out[q.id] = cleaned;
    } else {
      out[q.id] = value as AnswerValue;
    }
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

// ─── Группировка по секциям (docs/08 §5.1) ──────────────────────────────────

/** Группа вопросов с упорядоченным списком вопросов (для рендера). */
export interface QuestionGroup {
  /** Название секции; пустая строка — для вопросов без group. */
  name: string;
  questions: Question[];
}

/**
 * Группирует видимые вопросы по полю group, сохраняя:
 *   • порядок появления групп (как в схеме);
 *   • порядок «родитель → его условные дети сразу под ним» внутри группы.
 * Дочерние условные вопросы наследуют группу родителя в рендере (если у них
 * group не задан) — так каскад остаётся в одной секции.
 */
export function groupVisibleQuestions(
  schema: QuestionnaireSchema,
  visible: Set<string>,
): QuestionGroup[] {
  const ordered = orderVisibleQuestions(schema, visible);

  // Карта родитель-группы для наследования группы дочерними.
  const groupOf = new Map<string, string>();
  for (const q of schema.questions) {
    if (q.group) groupOf.set(q.id, q.group);
  }
  const parentGroup = new Map<string, string>();
  for (const q of schema.questions) {
    for (const cond of q.conditional ?? []) {
      for (const childId of cond.show) {
        if (!groupOf.has(childId)) {
          const g = groupOf.get(q.id) ?? parentGroup.get(q.id);
          if (g) parentGroup.set(childId, g);
        }
      }
    }
  }

  const groups: QuestionGroup[] = [];
  const indexByName = new Map<string, number>();
  for (const q of ordered) {
    const name = q.group ?? parentGroup.get(q.id) ?? "";
    let idx = indexByName.get(name);
    if (idx === undefined) {
      idx = groups.length;
      indexByName.set(name, idx);
      groups.push({ name, questions: [] });
    }
    groups[idx].questions.push(q);
  }
  return groups;
}

/**
 * Возвращает видимые вопросы в порядке «родитель → его дочерние сразу под ним».
 * Безусловные идут в порядке схемы; их дочерние вставляются следом (каскадно).
 */
export function orderVisibleQuestions(
  schema: QuestionnaireSchema,
  visible: Set<string>,
): Question[] {
  const byId = new Map(schema.questions.map((q) => [q.id, q]));
  const conditionalIds = collectConditionalIds(schema);
  const emitted = new Set<string>();
  const result: Question[] = [];

  const emitChildren = (parentId: string) => {
    const parent = byId.get(parentId);
    if (!parent?.conditional) return;
    for (const cond of parent.conditional) {
      for (const childId of cond.show) {
        const child = byId.get(childId);
        if (child && visible.has(childId) && !emitted.has(childId)) {
          emitted.add(childId);
          result.push(child);
          emitChildren(childId); // каскад
        }
      }
    }
  };

  for (const q of schema.questions) {
    if (conditionalIds.has(q.id)) continue; // дочерние вставляются родителем
    if (!visible.has(q.id) || emitted.has(q.id)) continue;
    emitted.add(q.id);
    result.push(q);
    emitChildren(q.id);
  }
  return result;
}

/** Множество id, являющихся дочерними хотя бы одного conditional. */
export function collectConditionalIds(schema: QuestionnaireSchema): Set<string> {
  const set = new Set<string>();
  for (const q of schema.questions) {
    for (const cond of q.conditional ?? []) {
      for (const id of cond.show) set.add(id);
    }
  }
  return set;
}

// ─── Хелперы «своего варианта» в multiselect ────────────────────────────────

/** Есть ли в multiselect-значении кастом-элемент. */
export function hasCustomItem(value: AnswerValue | undefined): boolean {
  return Array.isArray(value) && value.some((i) => isCustomAnswer(i));
}

/** Текст кастом-элемента multiselect (или ""). */
export function customItemText(value: AnswerValue | undefined): string {
  if (!Array.isArray(value)) return "";
  const found = value.find((i) => isCustomAnswer(i));
  return found && isCustomAnswer(found) ? found.custom_text : "";
}
