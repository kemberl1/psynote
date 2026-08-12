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
  llm_model_used?: string;
  tokens_used?: number;
  total?: number;
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
  | "UNAUTHORIZED"
  | "EMAIL_TAKEN"
  | "FORBIDDEN"
  | "UNKNOWN";

// ─── Аутентификация (docs/07 §2, docs/09) ──────────────────────────────────

export interface RegisterRequest {
  email: string;
  password: string;
  display_name?: string;
}

export interface RegisterResult {
  doctor_id: string;
  email: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export interface DoctorProfile {
  doctor_id: string;
  email: string;
  display_name: string;
  role: string;
}

// ─── Справочники и схема опросника (docs/07 §3, docs/06) ───────────────────

export interface DocumentType {
  code: string;
  title: string;
  is_active: boolean;
}

export type QuestionType =
  | "select"
  | "multiselect"
  | "text"
  | "number"
  | "boolean";

export interface QuestionOption {
  value: string;
  label: string;
  prompt?: string;
}

export interface Conditional {
  if_value: string;
  show: string[];
}

export interface Question {
  id: string;
  label: string;
  type: QuestionType;
  required: boolean;
  allow_custom: boolean;
  default?: unknown;
  group?: string;
  help?: string;
  options?: QuestionOption[];
  conditional?: Conditional[];
}

export interface QuestionnaireSchema {
  document_type: string;
  version: number;
  questions: Question[];
}

// ─── Генерация (docs/07 §5) ────────────────────────────────────────────────

export type MultiAnswerItem = string | CustomAnswer;
export type AnswerValue =
  | string
  | number
  | boolean
  | MultiAnswerItem[]
  | CustomAnswer
  | null;

export interface CustomAnswer {
  value: "__custom__";
  custom_text: string;
}

export type Answers = Record<string, AnswerValue>;

export interface GenerateRequest {
  document_type: string;
  answers: Answers;
  attachment_ids?: string[];
  options?: { stream?: boolean };
  /** Обновить существующую pending/failed запись. */
  request_id?: string;
  /** Привязать дневник к пакету. */
  parent_request_id?: string;
  /** Override заголовка (пакетные дни). */
  title_safe?: string;
}

export interface AnonymizationSummary {
  removed_count: number;
  removed_by_type: Record<string, number>;
}

export interface GenerateResult {
  request_id: string;
  content: string;
  status: string;
  anonymization: AnonymizationSummary;
}

export interface PendingRequest {
  document_type: string;
  title_safe: string;
  answers_anonymized?: Answers;
  parent_request_id?: string;
}

export interface PendingResult {
  request_id: string;
  document_type: string;
  title_safe: string;
  status: string;
}

export interface PatchRequestBody {
  title_safe?: string;
  status?: string;
  answers_anonymized?: Answers;
}

// ─── История запросов (docs/07 §6) ─────────────────────────────────────────

export interface HistoryItem {
  request_id: string;
  document_type: string;
  title_safe: string;
  llm_model_used: string;
  status: string;
  children_count?: number;
  created_at: string;
}

export interface HistoryChild {
  request_id: string;
  document_type: string;
  title_safe: string;
  status: string;
  content: string;
  created_at: string;
}

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
  children?: HistoryChild[];
}

export interface HistoryListResult {
  items: HistoryItem[];
  total: number;
}

// ─── Экспорт (docs/07 §7) ──────────────────────────────────────────────────

export type ExportFormat = "docx" | "pdf" | "txt";

export interface ExportRequest {
  format: ExportFormat;
  substitutions?: Record<string, string>;
}

export interface BatchExportRequest {
  format: ExportFormat;
  request_ids: string[];
  substitutions?: Record<string, string>;
}

// ─── Админка: загрузка документов (Этап 10, docs/07 §8) ─────────────────────

/** Результат загрузки документа через admin UI (POST /admin/documents). */
export interface AdminUploadResult {
  doc_id: string;
  status: string;
  original_filename: string;
  anonymizer_removed_count: number;
  removed_by_type: Record<string, number>;
  chunks_count: number;
  qdrant_ids: string[];
  error_message?: string;
}

/** Метаданные загруженного документа (GET /admin/documents). */
export interface AdminDocument {
  id: string;
  uploaded_by?: string;
  original_filename: string;
  status: string;
  anonymizer_removed_count: number;
  removed_by_type: Record<string, number>;
  chunks_count: number;
  qdrant_ids: string[];
  error_message?: string;
  created_at: string;
  updated_at: string;
}

/** Результат GET /admin/documents (список). */
export interface AdminDocumentListResult {
  items: AdminDocument[];
  total: number;
}
