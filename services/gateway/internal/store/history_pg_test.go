package store

import (
	"context"
	"os"
	"testing"
)

// Интеграционный тест на живом Postgres (схема из deploy/initdb):
//
//	PSYNOTE_TEST_DSN=postgres://aimed@127.0.0.1:55432/aimed go test ./internal/store/
//
// Без переменной пропускается.
func TestHistoryKeepsDiaryAndFeedback(t *testing.T) {
	dsn := os.Getenv("PSYNOTE_TEST_DSN")
	if dsn == "" {
		t.Skip("PSYNOTE_TEST_DSN не задан")
	}
	ctx := context.Background()

	// Дважды — DDL при старте gateway обязан быть идемпотентным.
	for i := 0; i < 2; i++ {
		r, err := NewPgxRepository(ctx, dsn)
		if err != nil {
			t.Fatalf("NewPgxRepository #%d: %v", i+1, err)
		}
		r.Close()
	}
	repo, err := NewPgxRepository(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	const email = "history-test@example.local"
	_, _ = repo.pool.Exec(ctx, `DELETE FROM doctor WHERE email = $1`, email)
	doctorID, err := repo.CreateDoctor(ctx, email, "x", "Тест", "doctor")
	if err != nil {
		t.Fatalf("CreateDoctor: %v", err)
	}
	// defer до repo.Close (LIFO): чистим, пока пул открыт.
	defer func() {
		_, _ = repo.pool.Exec(ctx, `DELETE FROM doctor WHERE id = $1`, doctorID)
	}()

	id, err := repo.SaveGeneration(ctx, GenerationRecord{
		DoctorID: &doctorID, DocumentType: "daily", TitleSafe: "День 1",
		Status: "done", ContentAnonymized: "текст v1",
		AnswersAnonymized: map[string]any{"__arc_context__": "бриф v1"},
	})
	if err != nil {
		t.Fatalf("SaveGeneration: %v", err)
	}

	if _, err := repo.UpsertFeedback(ctx, GenerationFeedback{
		RequestID: id, DoctorID: doctorID, Rating: 2, Comment: "замечание к v1",
	}); err != nil {
		t.Fatalf("UpsertFeedback v1: %v", err)
	}

	// Перегенерация: v1 уходит в версии.
	if err := repo.CompleteGeneration(ctx, id, &doctorID, GenerationRecord{
		DocumentType: "daily", TitleSafe: "День 1", Status: "done",
		ContentAnonymized: "текст v2",
		AnswersAnonymized: map[string]any{"__arc_context__": "бриф v2"},
	}); err != nil {
		t.Fatalf("CompleteGeneration: %v", err)
	}
	var oldText, oldBrief string
	if err := repo.pool.QueryRow(ctx, `
		SELECT content_anonymized, answers_anonymized->>'__arc_context__'
		FROM generated_document_version WHERE request_id = $1`, id,
	).Scan(&oldText, &oldBrief); err != nil {
		t.Fatalf("версия не сохранена: %v", err)
	}
	if oldText != "текст v1" || oldBrief != "бриф v1" {
		t.Fatalf("версия = %q / %q", oldText, oldBrief)
	}

	// Правка отзыва после перегенерации: старый комментарий — в историю.
	if _, err := repo.UpsertFeedback(ctx, GenerationFeedback{
		RequestID: id, DoctorID: doctorID, Rating: 4, Comment: "замечание к v2",
	}); err != nil {
		t.Fatalf("UpsertFeedback v2: %v", err)
	}
	var histComment, histSnapshot string
	if err := repo.pool.QueryRow(ctx, `
		SELECT h.comment, h.content_snapshot FROM generation_feedback_history h
		WHERE h.request_id = $1`, id,
	).Scan(&histComment, &histSnapshot); err != nil {
		t.Fatalf("история отзыва не сохранена: %v", err)
	}
	if histComment != "замечание к v1" || histSnapshot != "текст v1" {
		t.Fatalf("история = %q / %q", histComment, histSnapshot)
	}

	// Удаление дневника не удаляет отзыв.
	if err := repo.DeleteGeneration(ctx, id, &doctorID); err != nil {
		t.Fatalf("DeleteGeneration: %v", err)
	}
	items, _, err := repo.ListFeedback(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListFeedback: %v", err)
	}
	var found *AdminFeedbackItem
	for i := range items {
		if items[i].DoctorID == doctorID {
			found = &items[i]
		}
	}
	if found == nil {
		t.Fatal("отзыв пропал вместе с дневником")
	}
	if found.Comment != "замечание к v2" || found.ContentSnapshot != "текст v2" ||
		found.SnapshotBackfilled ||
		found.RequestID != "" || found.TitleSafe != "Дневник удалён" {
		t.Fatalf("отзыв после удаления: %+v", *found)
	}
}
