// Нарративный опросник для пакетной генерации — задаёт дугу периода, не
// поённый статус каждого дня. Три логические группы: клинический старт,
// динамика и лечение, финальное состояние.
import type { QuestionnaireSchema } from "../api/types";

export const BATCH_QUESTIONNAIRE: QuestionnaireSchema = {
  document_type: "batch",
  version: 3,
  questions: [
    // ── Клинический старт ─────────────────────────────────────────────────────
    {
      id: "patient_sex",
      label: "Пол",
      type: "select",
      required: true,
      allow_custom: false,
      group: "Клинический старт",
      help: "Нужен, чтобы согласовать род в тексте (он/она, упорядочен/упорядочена)",
      options: [
        { value: "male", label: "м (мальчик)" },
        { value: "female", label: "ж (девочка)" },
      ],
    },
    {
      id: "leading_syndrome",
      label: "Ведущий синдром при поступлении",
      type: "select",
      required: true,
      allow_custom: true,
      default: "anxious",
      group: "Клинический старт",
      help: "Задаёт клиническую «тональность» всех дневников",
      options: [
        { value: "behavioral", label: "поведенческих нарушений" },
        { value: "anxious", label: "тревожный" },
        { value: "depressive", label: "депрессивный" },
        { value: "psychomotor_aggression", label: "психомоторной расторможенности (с агрессией)" },
        { value: "psychomotor_autoaggression", label: "психомоторной расторможенности (с аутоагрессией)" },
        { value: "affective_volitional", label: "аффективно-волевой неустойчивости" },
        { value: "psychopathic", label: "психопатоподобный" },
        { value: "asthenic", label: "астенический" },
      ],
    },
    {
      id: "admission_severity",
      label: "Тяжесть состояния при поступлении",
      type: "select",
      required: true,
      allow_custom: false,
      default: "moderate",
      group: "Клинический старт",
      options: [
        { value: "mild", label: "лёгкая" },
        { value: "moderate", label: "средняя" },
        { value: "severe", label: "тяжёлая" },
      ],
    },
    {
      id: "diagnosis",
      label: "Основной диагноз",
      type: "text",
      required: false,
      allow_custom: true,
      group: "Клинический старт",
      help: "МКБ-10 код или описание — без ФИО пациента",
    },

    // ── Динамика и лечение ────────────────────────────────────────────────────
    {
      id: "overall_dynamics",
      label: "Общая динамика за период",
      type: "select",
      required: true,
      allow_custom: false,
      default: "positive",
      group: "Динамика и лечение",
      conditional: [
        { if_value: "positive", show: ["improvement_pace"] },
      ],
      options: [
        { value: "positive", label: "положительная" },
        { value: "stable", label: "стабильная, без выраженных изменений" },
        { value: "wavy", label: "волнообразная" },
        { value: "negative", label: "отрицательная" },
      ],
    },
    {
      id: "improvement_pace",
      label: "Темп улучшения",
      type: "select",
      required: false,
      allow_custom: false,
      default: "moderate",
      group: "Динамика и лечение",
      help: "Как быстро нарастает положительная динамика от первого дня к последнему",
      options: [
        { value: "fast", label: "быстрый — заметно с первых дней" },
        { value: "moderate", label: "умеренный — постепенное улучшение" },
        { value: "slow", label: "медленный — медленный старт, ускорение к концу" },
      ],
    },
    {
      id: "key_medications",
      label: "Ключевые препараты / схема",
      type: "text",
      required: false,
      allow_custom: true,
      group: "Динамика и лечение",
      help: "Укажите если важно отразить в дневниках. Без персональных данных.",
    },
    {
      id: "notable_events",
      label: "Значимые события периода",
      type: "multiselect",
      required: false,
      allow_custom: false,
      group: "Динамика и лечение",
      help: "ИИ распределит упоминания по подходящим дням",
      options: [
        { value: "specialist_consult", label: "консультации специалистов" },
        { value: "therapy_change", label: "изменение терапии" },
        { value: "exacerbation", label: "обострения / эпизоды ухудшения" },
        { value: "ecg_eeg", label: "ЭКГ / ЭЭГ" },
      ],
    },

    // ── Финальное состояние ───────────────────────────────────────────────────
    {
      id: "final_state",
      label: "Состояние к последнему дню периода",
      type: "text",
      required: false,
      allow_custom: true,
      group: "Финальное состояние",
      help: "Общее состояние пациента к концу выбранного периода формирования дневников для более точной калибровки динамики",
    },
  ],
};
