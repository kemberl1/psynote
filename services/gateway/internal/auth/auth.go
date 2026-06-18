// Package auth implements doctor authentication for the gateway (docs/09 §1):
// Argon2id password hashing (password.go) + HS256 JWT access tokens and opaque,
// revocable refresh tokens (jwt.go). Изоляция данных по doctor_id обязательна
// (docs/09 §3) — она обеспечивается middleware (internal/handlers/middleware.go),
// который кладёт проверенный doctor_id в context, и store-слоем, который
// фильтрует историю по владельцу.
//
// Состав пакета:
//   - password.go — HashPassword / VerifyPassword (Argon2id, PHC-формат);
//   - jwt.go      — TokenService (выпуск/валидация access JWT) +
//     GenerateRefreshToken / HashRefreshToken (opaque refresh).
//
// Этап 9 (аутентификация врачей) заменяет каркасную заглушку Этапа 1 реальной
// реализацией «золотого стандарта» без 2FA (docs/09 §1.2).
package auth
