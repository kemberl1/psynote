// QuestionnaireRenderer — рендер JSON-схемы опросника (docs/06, docs/08 §5.1).
// Держит общий стейт ответов, вычисляет видимость условных вопросов и рендерит
// их со смещением под родителем. Контролируемый: answers/onChange приходят из
// страницы (DiaryPage), чтобы submit и прогресс жили на уровне формы.
//
// Граница Этапа 6/7: базовая условная видимость работает (computeVisibleIds),
// порядок «родитель → его дети сразу под ним» обеспечивается renderOrder().
// Этап 7 углубит правила и добавит сложные виджеты — без смены этого контракта.
import { useMemo } from "react";
import type {
    AnswerValue,
    Answers,
    Question,
    QuestionnaireSchema,
} from "../../api/types";
import { computeVisibleIds } from "../../lib/questionnaire";
import { QuestionField } from "./QuestionField";
import "./questionnaire.css";

interface QuestionnaireRendererProps {
  schema: QuestionnaireSchema;
  answers: Answers;
  onChange: (id: string, value: AnswerValue) => void;
}

export function QuestionnaireRenderer({
  schema,
  answers,
  onChange,
}: QuestionnaireRendererProps) {
  const visible = useMemo(
    () => computeVisibleIds(schema, answers),
    [schema, answers],
  );

  // Множество «дочерних» (условных) вопросов — для смещённого рендера.
  const conditionalIds = useMemo(() => {
    const set = new Set<string>();
    for (const q of schema.questions) {
      for (const cond of q.conditional ?? []) {
        for (const id of cond.show) set.add(id);
      }
    }
    return set;
  }, [schema]);

  // Порядок: каждый видимый родитель, затем сразу его видимые дочерние вопросы.
  const order = useMemo(
    () => renderOrder(schema, visible, conditionalIds),
    [schema, visible, conditionalIds],
  );

  return (
    <div className="qn">
      {order.map((q) => (
        <QuestionField
          key={q.id}
          question={q}
          value={answers[q.id]}
          conditional={conditionalIds.has(q.id)}
          onChange={(value) => onChange(q.id, value)}
        />
      ))}
    </div>
  );
}

/**
 * Возвращает видимые вопросы в порядке «родитель → его дочерние сразу под ним».
 * Безусловные идут в порядке схемы; их дочерние вставляются следом.
 */
function renderOrder(
  schema: QuestionnaireSchema,
  visible: Set<string>,
  conditionalIds: Set<string>,
) {
  const byId = new Map(schema.questions.map((q) => [q.id, q]));
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
