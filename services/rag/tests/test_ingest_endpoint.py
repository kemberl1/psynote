"""Юнит-тесты POST /ingest — загрузка документов через UI (Этап 10).

Проверяем:
  (а) успешная загрузка .docx → extract → anonymize → chunk → embed → upsert;
  (б) неподдерживаемый формат → 400;
  (в) пустой файл → 400;
  (г) PII blocked (fail-closed) → 422;
  (д) нет валидных чанков → 422;
  (е) source="user_upload" в payload.

Запуск: cd services/rag && python -m pytest tests/test_ingest_endpoint.py -q
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient

from app.anonymizer_client import AnonymizeResult
from app.chunking import Chunk
from app.main import app


# ─── Test client ──────────────────────────────────────────────────────────────

@pytest.fixture()
def client():
    """FastAPI test client (no network, no Qdrant, no gateway)."""
    return TestClient(app)


# ─── Хелпер: fake .docx bytes (минимальный ZIP для расширения) ──────────

def _fake_docx_bytes() -> bytes:
    """Корректный ZIP-архив с XML — минимальный .docx."""
    import io
    import zipfile
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("[Content_Types].xml",
                    '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
                    '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
                    '<Default Extension="xml" ContentType="application/xml"/>'
                    '<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>'
                    '</Types>')
        zf.writestr("_rels/.rels",
                    '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
                    '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>'
                    '</Relationships>')
        zf.writestr("word/_rels/document.xml.rels",
                    '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>')
        zf.writestr("word/document.xml",
                    '<?xml version="1.0"?>'
                    '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
                    '<w:body><w:p><w:r><w:t>25.01.2024. Дневник. Настроение сниженное.</w:t></w:r></w:p></w:body>'
                    '</w:document>')
    return buf.getvalue()


def _fake_chunks():
    return [
        Chunk(text="Настроение сниженное.", doc_type="daily", section="subjective",
              syndrome="", diagnosis_class="", dynamics=""),
    ]


def _fake_anon_result(passed: bool = True, content: str = "Настроение сниженное.",
                      removed_count: int = 1, reason: str = "ok",
                      removed_by_type: dict | None = None) -> AnonymizeResult:
    return AnonymizeResult(
        passed=passed, content=content, removed_count=removed_count,
        reason=reason, removed_by_type=removed_by_type or {"ФИО": 1},
    )


# ─── Тесты ────────────────────────────────────────────────────────────────────

class TestIngestEndpoint:
    """Набор тестов для POST /ingest."""

    # /ingest uses lazy imports inside the function body, so we patch
    # the SOURCE modules (app.extractors, app.anonymizer_client, etc.)
    # not app.main.*.

    @patch("app.qdrant_store.QdrantStore")
    @patch("app.embeddings.Embedder")
    @patch("app.chunking.chunk_document")
    @patch("app.anonymizer_client.AnonymizerClient")
    @patch("app.extractors.extract_text")
    def test_ingest_success(
        self, mock_extract, mock_anon_cls, mock_chunk, mock_embed_cls, mock_store_cls,
        client,
    ):
        """Полный цикл: extract → anonymize → chunk → embed → upsert → 200."""
        mock_extract.return_value = "25.01.2024. Настроение сниженное."

        mock_anon = MagicMock()
        mock_anon.anonymize.return_value = _fake_anon_result()
        mock_anon_cls.return_value = mock_anon

        mock_chunk.return_value = _fake_chunks()

        mock_embed = MagicMock()
        mock_embed.embed_passages.return_value = [[0.1] * 10]
        mock_embed.dimension = 10
        mock_embed_cls.return_value = mock_embed

        mock_store = MagicMock()
        mock_store.upsert.return_value = 1
        mock_store_cls.return_value = mock_store

        docx = _fake_docx_bytes()
        resp = client.post("/ingest", files={
            "file": ("diary.docx", docx,
                     "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
        })

        assert resp.status_code == 200, f"status={resp.status_code} body={resp.text}"
        data = resp.json()["data"]
        assert data["status"] == "ingested"
        assert data["chunks_count"] == 1
        assert data["anonymizer_removed_count"] == 1
        assert "ФИО" in data["removed_by_type"]
        assert isinstance(data["qdrant_ids"], list)
        assert len(data["qdrant_ids"]) == 1

    def test_ingest_unsupported_format(self, client):
        """Файл .xlsx → 400 BAD_REQUEST."""
        resp = client.post("/ingest", files={
            "file": ("data.xlsx", b"PK\x03\x04content",
                     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
        })
        assert resp.status_code == 400
        body = resp.json()
        assert body["error"]["code"] == "BAD_REQUEST"

    def test_ingest_empty_file(self, client):
        """Пустой файл → 400 BAD_REQUEST."""
        resp = client.post("/ingest", files={
            "file": ("empty.docx", b"",
                     "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
        })
        assert resp.status_code == 400

    @patch("app.anonymizer_client.AnonymizerClient")
    @patch("app.extractors.extract_text")
    def test_ingest_pii_blocked(self, mock_extract, mock_anon_cls, client):
        """Anonymizer gate не пропустил → 422 PII_DETECTED."""
        mock_extract.return_value = "Иванов Иван Иванович, дата рождения 01.01.1990"

        mock_anon = MagicMock()
        mock_anon.anonymize.return_value = _fake_anon_result(
            passed=False, content="", reason="pii_detected")
        mock_anon_cls.return_value = mock_anon

        resp = client.post("/ingest", files={
            "file": ("pii.docx", _fake_docx_bytes(),
                     "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
        })
        assert resp.status_code == 422
        body = resp.json()
        assert body["error"]["code"] == "PII_DETECTED"

    @patch("app.chunking.chunk_document")
    @patch("app.anonymizer_client.AnonymizerClient")
    @patch("app.extractors.extract_text")
    def test_ingest_no_chunks(self, mock_extract, mock_anon_cls, mock_chunk, client):
        """Anonymizer прошёл, но chunk_document → пустой список → 422 NO_CHUNKS."""
        mock_extract.return_value = "Привет"
        mock_anon = MagicMock()
        mock_anon.anonymize.return_value = _fake_anon_result(content="Привет")
        mock_anon_cls.return_value = mock_anon
        mock_chunk.return_value = []

        resp = client.post("/ingest", files={
            "file": ("tiny.docx", _fake_docx_bytes(),
                     "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
        })
        assert resp.status_code == 422
        body = resp.json()
        assert body["error"]["code"] == "NO_CHUNKS"

    @patch("app.qdrant_store.QdrantStore")
    @patch("app.embeddings.Embedder")
    @patch("app.chunking.chunk_document")
    @patch("app.anonymizer_client.AnonymizerClient")
    @patch("app.extractors.extract_text")
    def test_ingest_source_tag_is_user_upload(
        self, mock_extract, mock_anon_cls, mock_chunk, mock_embed_cls, mock_store_cls,
        client,
    ):
        """Upsert должен использовать source="user_upload" (не corpus)."""
        mock_extract.return_value = "Текст дневника"
        mock_anon = MagicMock()
        mock_anon.anonymize.return_value = _fake_anon_result(
            content="Текст дневника")
        mock_anon_cls.return_value = mock_anon

        chunks = [Chunk(text="Текст дневника", doc_type="daily", section="subjective",
                        syndrome="", diagnosis_class="", dynamics="")]
        mock_chunk.return_value = chunks

        mock_embed = MagicMock()
        mock_embed.embed_passages.return_value = [[0.1] * 10]
        mock_embed.dimension = 10
        mock_embed_cls.return_value = mock_embed

        mock_store = MagicMock()
        mock_store.upsert.return_value = 1
        mock_store_cls.return_value = mock_store

        resp = client.post("/ingest", files={
            "file": ("doc.docx", _fake_docx_bytes(),
                     "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
        })
        assert resp.status_code == 200

        # Проверяем, что upsert вызван с payload имеющим source="user_upload"
        call_args = mock_store.upsert.call_args
        points = call_args[0][0]  # first positional arg: list[PointRecord]
        assert len(points) >= 1
        assert points[0].payload["source"] == "user_upload"
