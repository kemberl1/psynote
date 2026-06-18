// Единая модель ошибок API + человекочитаемые сообщения (docs/07 §1, docs/08 §7).
import type { ApiErrorCode } from "./types";

/**
 * ApiError — нормализованная ошибка любого вызова API. Несёт контрактный код
 * (docs/07 §1), HTTP-статус и техническое сообщение. Для UI используйте
 * userMessage(error) — он маппит код в дружелюбный русский текст (docs/08 §7).
 */
export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;
  readonly requestId?: string;

  constructor(
    code: ApiErrorCode,
    message: string,
    status: number,
    requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.requestId = requestId;
  }
}

/** Заголовок + текст для баннера ошибки (docs/08 §7). */
export interface FriendlyError {
  title: string;
  detail: string;
  /** tone влияет на цвет баннера: спокойный (privacy) vs ошибка. */
  tone: "danger" | "warning";
}

/**
 * Маппинг контрактных кодов в дружелюбные сообщения (docs/08 §7).
 * PII_DETECTED — спокойный «warning»-тон (это не сбой, а защита приватности).
 */
export function friendlyError(err: unknown): FriendlyError {
  const code: ApiErrorCode =
    err instanceof ApiError ? err.code : "UNKNOWN";

  switch (code) {
    case "PII_DETECTED":
      return {
        title: "Обнаружены персональные данные",
        detail:
          "Запрос не выполнен: во введённом тексте найдены персональные данные. " +
          "Уберите ФИО, даты, адреса и контакты — система работает только с обезличенными данными.",
        tone: "warning",
      };
    case "LLM_UNAVAILABLE":
    case "SERVICE_UNAVAILABLE":
      return {
        title: "Сервис генерации недоступен",
        detail: "Сервис генерации временно недоступен. Попробуйте позже.",
        tone: "danger",
      };
    case "INVALID_DOCUMENT_TYPE":
      return {
        title: "Неизвестный тип документа",
        detail: "Выбранный тип документа не поддерживается.",
        tone: "danger",
      };
    case "BAD_REQUEST":
      return {
        title: "Некорректный запрос",
        detail:
          err instanceof ApiError && err.message
            ? err.message
            : "Проверьте заполнение формы и повторите.",
        tone: "danger",
      };
    case "NOT_FOUND":
      return {
        title: "Запись не найдена",
        detail: "Запрашиваемая запись истории не существует или была удалена.",
        tone: "warning",
      };
    case "UNAUTHORIZED":
      return {
        title: "Требуется вход",
        detail: "Сессия истекла или вы не авторизованы. Войдите снова.",
        tone: "warning",
      };
    case "EMAIL_TAKEN":
      return {
        title: "Email уже занят",
        detail: "Аккаунт с таким email уже существует. Попробуйте войти.",
        tone: "warning",
      };
    case "NETWORK":
      return {
        title: "Нет соединения",
        detail:
          "Не удалось связаться с сервером. Проверьте подключение и повторите.",
        tone: "danger",
      };
    default:
      return {
        title: "Что-то пошло не так",
        detail: "Произошла непредвиденная ошибка. Попробуйте ещё раз.",
        tone: "danger",
      };
  }
}
