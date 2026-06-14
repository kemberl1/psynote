// QuestionnaireRenderer — рендер JSON-схемы опросника (docs/06, docs/08 §5.1).
// Держит общий стейт ответов снаружи (контролируемый), вычисляет видимость
// условных вопросов и группирует их по логическим секциям. Условные дочерние
// вопросы рендерятся со смещением сразу под родителем (renderOrder в lib).
//
// Этап 7: полная условная логика (каскад, multiselect-триггеры, «свой вариант»)
// живёт в lib/questionnaire. Здесь — группировка по секциям, плавное появление
// условных вопросов и проброс «свой вариант» в QuestionField.
import { useMemo } from "react";
import type {
  AnswerValue,
  Answers,
  QuestionnaireSchema,
} from "../../api/types";
import {
  collectConditionalIds,
  computeVisibleIds,
  groupVisibleQuestions,
} from "../../lib/questionnaire";
import { QuestionField } from "./QuestionField";
import "./questionnaire.css";

interface QuestionnaireRendererProps {
  schema: QuestionnaireSchema;
  answers: Answers;
  onChange: (id: string, value: AnswerValue) => void;
  /** id вопросов, которые нужно подсветить как незаполненные (валидация). */
  invalidIds?: Set<string>;
}

export function QuestionnaireRenderer({
  schema,
  answers,
  onChange,
  invalidIds,
}: QuestionnaireRendererProps) {
  const visible = useMemo(
    () => computeVisibleIds(schema, answers),
    [schema, answers],
  );

  const conditionalIds = useMemo(
    () => collectConditionalIds(schema),
    [schema],
  );

  // Видимые вопросы, сгруппированные по секциям, с порядком «родитель→дети».
  const groups = useMemo(
    () => groupVisibleQuestions(schema, visible),
    [schema, visible],
  );

  return (
    <div className="qn">
      {groups.map((group) => (
        <section className="qn-group" key={group.name || "_default"}>
          {group.name && (
            <h3 className="qn-group__title">{group.name}</h3>
          )}
          <div className="qn-group__fields">
            {group.questions.map((q) => (
              <QuestionField
                key={q.id}
                question={q}
                value={answers[q.id]}
                conditional={conditionalIds.has(q.id)}
                invalid={invalidIds?.has(q.id) ?? false}
                onChange={(value) => onChange(q.id, value)}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
