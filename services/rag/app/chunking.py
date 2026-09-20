"""Структурно-осознанный чанкинг ОБЕЗЛИЧЕННЫХ медицинских дневников (docs/03 §4).

Вход — ТОЛЬКО обезличенный текст (после гейта gateway, docs/04). Здесь нет и не
должно быть работы с ПДн: к этому модулю текст приходит уже с плейсхолдерами
([ДАТА], [ПАЦИЕНТ], [ФИО_ВРАЧА] и т.д.).

Стратегия (docs/03 §4):
  - Чанк = одна логическая запись/секция, а не «слепое нарезание по N символов».
  - Два типа записей в дневниках:
      * daily    — ежедневная запись (обычно один абзац: статус + подпись);
      * exam_10d — расширенный осмотр раз в 10 дней (структурированные секции:
                   Жалобы / Анамнез / Физикальное / Неврологический статус /
                   Психический статус / Диагноз / Назначения / Этапный эпикриз ...).
  - Расширенный осмотр режется по смысловым секциям с общими метаданными; если
    секция/запись больше chunk_max_chars — мягко досекаем с перекрытием.

Метаданные чанка (docs/05 §3.2): doc_type, section, syndrome?, diagnosis_class?,
dynamics?, source. ФИО/даты тут уже отсутствуют (обезличены гейтом).
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

from app.config import Settings
from app.questionnaire import normalize_syndrome

# ─── Заголовки секций расширенного осмотра (docs/03 §4) → код section ─────────
_SECTION_PATTERNS: list[tuple[str, re.Pattern[str]]] = [
    ("complaints", re.compile(r"^\s*жалоб", re.IGNORECASE)),
    ("anamnesis", re.compile(r"^\s*анамнез", re.IGNORECASE)),
    ("somatic", re.compile(r"^\s*(физикальн|соматическ|объективн)", re.IGNORECASE)),
    ("neuro", re.compile(r"^\s*неврологическ", re.IGNORECASE)),
    ("psych_status", re.compile(r"^\s*психическ(ий|ое)?\s*статус", re.IGNORECASE)),
    ("diagnosis", re.compile(r"^\s*(диагноз|основное заболевание|сопутств)", re.IGNORECASE)),
    ("assignments", re.compile(
        r"^\s*(назначени|план обследования|план лечения)", re.IGNORECASE)),
    ("interventions", re.compile(
        r"^\s*(выполнены|медицинские вмешательства)", re.IGNORECASE)),
    ("epicrisis", re.compile(r"^\s*(этапный\s+)?эпикриз", re.IGNORECASE)),
]

# Маркеры начала ежедневной записи: дата уже обезличена → плейсхолдер [ДАТА]/[ПЕРИОД].
_DATE_PLACEHOLDER_RE = re.compile(r"^\s*\[(ДАТА|ПЕРИОД)\]")

# Сигнал расширенного осмотра — присутствие нескольких именованных секций.
# Осмотр за 10 дней отличают ДВЕ вещи: подпись заведующего отделением и
# этапный эпикриз. «Жалобы» и «анамнез» есть и в обычном ежедневном бланке —
# по ним 5807 записей корпуса ошибочно считались осмотрами (прод, 20.09).
_EXAM_SIGNAL_RE = re.compile(
    r"(этапн\w*\s+эпикриз|совместно\s+с\s+заведующ|"
    r"заведующ\w*\s+отделением|осмотр\s+за\s+10\s+дн)", re.IGNORECASE)

# ─── Эвристики метаданных (опциональные поля docs/05) ────────────────────────
_ICD_RE = re.compile(r"\b([A-ZА-Я]\d{2})(?:\.\d+)?\b")
_SYNDROME_RE = re.compile(
    r"(тревожно-депрессивн\w*|депрессивн\w*|психопатоподобн\w*|"
    r"астеническ\w*|тревожн\w*|неврозоподобн\w*|апато-абулическ\w*|"
    r"маниакальн\w*|кататоническ\w*|психомоторн\w*|поведенческ\w*|"
    r"аффективно-волев\w*)", re.IGNORECASE)
_DYNAMICS_RULES: list[tuple[str, re.Pattern[str]]] = [
    ("улучшение", re.compile(
        r"(улучшени|положительн\w+ динамик|стабилизац|редукц)", re.IGNORECASE)),
    ("ухудшение", re.compile(r"(ухудшени|отрицательн\w+ динамик|нарастани)", re.IGNORECASE)),
    ("без_динамики", re.compile(
        r"(без\s+(существенн\w+\s+)?динамик|без\s+измен)", re.IGNORECASE)),
]


@dataclass
class Chunk:
    """Обезличенный чанк + метаданные для Qdrant payload (docs/05 §3.2)."""

    text: str
    doc_type: str  # daily | exam_10d
    section: str   # psych_status | somatic | neuro | epicrisis | full | ...
    syndrome: str | None = None
    diagnosis_class: str | None = None
    dynamics: str | None = None
    extra: dict = field(default_factory=dict)


def chunk_document(anonymized_text: str, settings: Settings) -> list[Chunk]:
    """Разбить ОБЕЗЛИЧЕННЫЙ текст документа на чанки с метаданными.

    Документ может содержать много записей (сборник дневников одного пациента).
    Разбиваем на записи по маркерам [ДАТА]/[ПЕРИОД], каждую классифицируем
    как daily или exam_10d и режем соответствующе.
    """
    records = _split_into_records(anonymized_text)
    doc_meta = _document_meta(anonymized_text)
    chunks: list[Chunk] = []
    for record in records:
        record = record.strip()
        if len(record) < settings.chunk_min_chars:
            continue
        if _EXAM_SIGNAL_RE.search(record):
            chunks.extend(_chunk_exam(record, settings, doc_meta))
        else:
            chunks.extend(_chunk_daily(record, settings, doc_meta))
    return chunks


# ─── Разбиение на записи ──────────────────────────────────────────────────────
def _split_into_records(text: str) -> list[str]:
    """Резать на записи по строкам, начинающимся с плейсхолдера даты.

    Если дат-плейсхолдеров нет (текст без распознанных дат) — fallback на
    разбиение по пустым строкам (абзацы).
    """
    lines = text.splitlines()
    records: list[str] = []
    current: list[str] = []
    seen_date = False

    for line in lines:
        if _DATE_PLACEHOLDER_RE.match(line):
            seen_date = True
            if current:
                records.append("\n".join(current))
                current = []
        current.append(line)
    if current:
        records.append("\n".join(current))

    if not seen_date:
        # Fallback: абзацы по пустым строкам.
        blocks = re.split(r"\n\s*\n", text)
        return [b for b in blocks if b.strip()]
    return [r for r in records if r.strip()]


def _count_sections(record: str) -> int:
    count = 0
    for line in record.splitlines():
        for _code, pattern in _SECTION_PATTERNS:
            if pattern.match(line):
                count += 1
                break
    return count


# ─── Ежедневная запись → один чанк (с мягким досеканием) ──────────────────────
def _chunk_daily(record: str, settings: Settings,
                 doc_meta: dict | None = None) -> list[Chunk]:
    """Ежедневная запись: по разделам бланка, если они есть.

    Отдельный чанк «психический статус» — то, ради чего RAG и нужен: именно
    его стиль подмешивается в промпт.
    """
    meta = _merge_meta(_extract_meta(record), doc_meta)
    chunks: list[Chunk] = []
    if _count_sections(record) >= 2:
        for section_code, section_text in _split_exam_sections(record):
            section_text = section_text.strip()
            if len(section_text) < settings.chunk_min_chars:
                continue
            for piece in _soft_split(section_text, settings):
                chunks.append(Chunk(
                    text=piece, doc_type="daily", section=section_code,
                    syndrome=meta["syndrome"], diagnosis_class=meta["icd"],
                    dynamics=meta["dynamics"]))
    if not chunks:
        chunks = [
            Chunk(text=p, doc_type="daily", section="full",
                  syndrome=meta["syndrome"], diagnosis_class=meta["icd"],
                  dynamics=meta["dynamics"])
            for p in _soft_split(record, settings)
            if not _is_document_header(p)
        ]
    return chunks


_HEADER_RE = re.compile(
    r"основное заболевание|история болезни|сборник|лист назначений", re.IGNORECASE)


def _is_document_header(piece: str) -> bool:
    """Шапка документа — не образец стиля: клинического наблюдения в ней нет."""
    return len(piece) < 160 and bool(_HEADER_RE.search(piece))


# ─── Расширенный осмотр → чанки по секциям ────────────────────────────────────
def _chunk_exam(record: str, settings: Settings,
                doc_meta: dict | None = None) -> list[Chunk]:
    meta = _merge_meta(_extract_meta(record), doc_meta)
    sections = _split_exam_sections(record)
    chunks: list[Chunk] = []
    for section_code, section_text in sections:
        section_text = section_text.strip()
        if len(section_text) < settings.chunk_min_chars:
            continue
        for piece in _soft_split(section_text, settings):
            chunks.append(Chunk(
                text=piece, doc_type="exam_10d", section=section_code,
                syndrome=meta["syndrome"], diagnosis_class=meta["icd"],
                dynamics=meta["dynamics"],
            ))
    if not chunks:
        # Целиком как один чанк, если секции не распознались.
        for piece in _soft_split(record, settings):
            chunks.append(Chunk(
                text=piece, doc_type="exam_10d", section="full",
                syndrome=meta["syndrome"], diagnosis_class=meta["icd"],
                dynamics=meta["dynamics"]))
    return chunks


def _split_exam_sections(record: str) -> list[tuple[str, str]]:
    """Разбить осмотр на (section_code, text) по заголовкам секций."""
    lines = record.splitlines()
    sections: list[tuple[str, list[str]]] = []
    current_code = "full"
    current_lines: list[str] = []

    for line in lines:
        matched = None
        for code, pattern in _SECTION_PATTERNS:
            if pattern.match(line):
                matched = code
                break
        if matched:
            if current_lines:
                sections.append((current_code, current_lines))
            current_code = matched
            current_lines = [line]
        else:
            current_lines.append(line)
    if current_lines:
        sections.append((current_code, current_lines))

    return [(code, "\n".join(ls)) for code, ls in sections]


# ─── Мягкое досекание длинных фрагментов с перекрытием ────────────────────────
def _soft_split(text: str, settings: Settings) -> list[str]:
    max_chars = settings.chunk_max_chars
    if len(text) <= max_chars:
        return [text]

    overlap = settings.chunk_overlap_chars
    pieces: list[str] = []
    start = 0
    n = len(text)
    while start < n:
        end = min(start + max_chars, n)
        # Стараемся резать по границе предложения/строки.
        if end < n:
            window = text[start:end]
            cut = max(window.rfind(". "), window.rfind("\n"))
            if cut > max_chars // 2:
                end = start + cut + 1
        piece = text[start:end].strip()
        if piece:
            pieces.append(piece)
        if end >= n:
            break
        start = max(end - overlap, start + 1)
    return pieces


def _document_meta(text: str) -> dict:
    """Метаданные всего документа: код МКБ и синдром из шапки истории болезни.

    В сборнике дневников одного пациента диагноз написан один раз, а записи
    дня его не повторяют. Берём метку документа ТОЛЬКО если она однозначна:
    два разных класса в файле — лучше без метки, чем с чужой.
    """
    classes = {
        f"F{raw[1]}x"
        for raw in (m.group(1) for m in _ICD_RE.finditer(text))
        if raw[0].upper() in ("F", "Ф") and raw[1:].isdigit()
    }
    syndromes = {
        normalize_syndrome(m.group(1))
        for m in _SYNDROME_RE.finditer(text)
    } - {None, ""}
    return {
        "icd": next(iter(classes)) if len(classes) == 1 else None,
        "syndrome": next(iter(syndromes)) if len(syndromes) == 1 else None,
        "dynamics": None,
    }


def _merge_meta(record_meta: dict, doc_meta: dict | None) -> dict:
    """Метка записи важнее метки документа; пустые поля добираем из документа."""
    if not doc_meta:
        return record_meta
    return {
        key: record_meta.get(key) or doc_meta.get(key)
        for key in ("icd", "syndrome", "dynamics")
    }


# ─── Извлечение опциональных метаданных ───────────────────────────────────────
def _extract_meta(text: str) -> dict:
    icd_match = _ICD_RE.search(text)
    icd_class = None
    if icd_match:
        # Верхний уровень МКБ: первая буква + первая цифра + 'x' (напр. F9x).
        # ТОЛЬКО F/Ф коды (психиатрическая классификация) → diagnosis_class;
        # R, J, E и др. — сопутствующие/соматические, не маппятся в class.
        raw = icd_match.group(1)
        first = raw[0].upper()
        if first in ("F", "Ф") and raw[1:].isdigit():
            icd_class = f"F{raw[1]}x"

    syndrome_match = _SYNDROME_RE.search(text)
    syndrome = normalize_syndrome(
        syndrome_match.group(1)) if syndrome_match else None

    dynamics = None
    for label, pattern in _DYNAMICS_RULES:
        if pattern.search(text):
            dynamics = label
            break

    return {"icd": icd_class, "syndrome": syndrome, "dynamics": dynamics}
