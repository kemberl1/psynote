// Типы контракта API gateway (docs/07_api_contract.md).
// Имена полей строго соответствуют реальным Go-структурам (см.
// services/gateway/internal/{catalog,store,handlers}). Никаких ПДн на фронте.

/** Конверт ответа (docs/07 §1). Либо data, либо error. */
export interface Envelope<TData> {
  meta: Meta;
  data?: TData;
  error?: ApiErrorBody;
}

/** meta-блок конверта (docs/07 §1, §3, §5, §6). */
export interface Meta {
  request_id?: string;
  ts?: string;
  /** Метаданные генерации (docs/07 §5). */
  llm_model_used?: string;
  tokens_used?: number;
  /** Пагинация истории (docs/07 §6). */
  total?: number;
  /** Версия схемы опросника (docs/07 §3). */
  version?: number;
}

/** Тело ошибки (docs/07 §1). */
export interface ApiErrorBody {
  code: string;
  message: string;
}

/** Известные коды ошибок контракта (docs/07 §1). */
export type ApiErrorCode =
  | "BAD_REQUEST"
  | "INVALID_DOCUMENT_TYPE"
  | "PII_DETECTED"
  | "LLM_UNAVAILABLE"
  | "NOT_FOUND"
  | "SERVICE_UNAVAILABLE"
  | "INTERNAL"
  | "NETWORK"
  // Аутентификация (Этап 9, docs/07 §2, docs/09).
  | "UNAUTHORIZED"
  | "EMAIL_TAKEN"
  | "UNKNOWN";

// ─── Аутентификация (docs/07 §2, docs/09) ──────────────────────────────────

/** Тело POST /auth/register. */
export interface RegisterRequest {
  email: string;
  password: string;
  display_name?: string;
}

/** Данные ответа POST /auth/register. */
export interface RegisterResult {
  doctor_id: string;
  email: string;
}

/** Тело POST /auth/login. */
export interface LoginRequest {
  email: string;
  password: string;
}

/** Пара токенов (login/refresh). access — короткоживущий JWT, refresh — opaque. */
export interface TokenPair {
  access_token: string;
  refresh_token: string;
  /** Время жизни access-токена в секундах (docs/07 §2). */
  expires_in: number;
}

/** Профиль текущего врача (GET /auth/me). Данные врача, не ПДн пациента. */
export interface DoctorProfile {
  doctor_id: string;
  email: string;
  display_name: string;
  role: string;
}

// ─── Справочники и схема опросника (docs/07 §3, docs/06) ───────────────────

/** Тип документа (GET /document-types). */
export interface DocumentType {
  code: string;
  title: string;
  is_active: boolean;
}

/** Тип поля вопроса (docs/06 §3). */
export type QuestionType =
  | "select"
  | "multiselect"
  | "text"
  | "number"
  | "boolean";

/** Опция select/multiselect (docs/06 §3). `prompt` фронтом не используется. */
export interface QuestionOption {
  value: string;
  label: string;
  prompt?: string;
}

/** Условная логика: какие вопросы открыть при значении (docs/06 §3). */
export interface Conditional {
  if_value: string;
  show: string[];
}

/** Один вопрос опросника (docs/06 §3). */
export interface Question {
  id: string;
  label: string;
  type: QuestionType;
  required: boolean;
  allow_custom: boolean;
  default?: unknown;
  /** Логическая секция для группировки в UI (docs/08 §5.1). Опционально. */
  group?: string;
  /** Короткая подсказка под вопросом. Опционально. */
  help?: string;
  options?: QuestionOption[];
  conditional?: Conditional[];
}

/** Схема опросника для типа документа (GET /questionnaire). */
export interface QuestionnaireSchema {
  document_type: string;
  version: number;
  questions: Question[];
}

// ─── Генерация (docs/07 §5) ────────────────────────────────────────────────

/**
 * Значение одного ответа опросника. Скаляр (select/text/number/boolean),
 * массив (multiselect) или объект «свой вариант» (docs/06 §1.4):
 * { value: "__custom__", custom_text: "..." }.
 *
 * Multiselect может содержать как коды опций (string), так и «свои варианты»
 * (CustomAnswer) — оба обрабатываются маппингом RAG (iter_free_text/_normalize).
 */
export type MultiAnswerItem = string | CustomAnswer;

export type AnswerValue =
  | string
  | number
  | boolean
  | MultiAnswerItem[]
  | CustomAnswer
  | null;

/** «Свой вариант» — свободный ввод (анонимизируется на gateway). */
export interface CustomAnswer {
  value: "__custom__";
  custom_text: string;
}

/** Карта ответов опросника (id вопроса → значение). */
export type Answers = Record<string, AnswerValue>;

/** Тело POST /generate (docs/07 §5). */
export interface GenerateRequest {
  document_type: string;
  answers: Answers;
  attachment_ids?: string[];
  options?: { stream?: boolean };
}

/**
 * Сводка обезличивания свободного ввода врача (docs/07 §5, docs/04 §7).
 * ТОЛЬКО счётчики/категории — НИКОГДА значения ПДн.
 */
export interface AnonymizationSummary {
  removed_count: number;
  removed_by_type: Record<string, number>;
}

/** Данные успешной генерации (docs/07 §5). */
export interface GenerateResult {
  request_id: string;
  content: string;
  status: string;
  anonymization: AnonymizationSummary;
}

// ─── История запросов (docs/07 §6) ─────────────────────────────────────────

/** Элемент списка истории (GET /requests). */
export interface HistoryItem {
  request_id: string;
  document_type: string;
  title_safe: string;
  llm_model_used: string;
  status: string;
  created_at: string;
}

/** Полная запись истории (GET /requests/{id}). Только обезличенные данные. */
export interface HistoryDetail {
  request_id: string;
  document_type: string;
  answers_anonymized: Answers;
  content: string;
  title_safe: string;
  llm_model_used: string;
  status: string;
  anonymizer_removed_count: number;
  created_at: string;
}

/** Список истории + total из meta (для пагинации). */
export interface HistoryListResult {
  items: HistoryItem[];
  total: number;
}

// ─── Экспорт (docs/07 §7) ──────────────────────────────────────────────────

/** Формат экспорта документа (docs/07 §7). */
export type ExportFormat = "docx" | "pdf" | "txt";

/**
 * Тело POST /requests/{id}/export (docs/07 §7). substitutions — локальная
 * подстановка реальных значений плейсхолдеров (применяется gateway в памяти,
 * не сохраняется). Может быть опущена.
 */
export interface ExportRequest {
  format: ExportFormat;
  substitutions?: Record<string, string>;
}
