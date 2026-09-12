"""Постобработка черновика дневника: бланк МИС, язык, терапия.

Модель часто кладёт инъекции и смену схемы в психический статус и в
«Назначения». По отделению: «Назначения» всегда «см. лист назначений»;
инъекции и коррекция терапии — в «План лечения»; в статусе остаётся
только наблюдаемое поведение.
"""

from __future__ import annotations

import re

from app.typos import fix_obvious_typos

_PRESCRIPTION_DEFAULT = "см. лист назначений"
_PLAN_EMPTY = "без дополнений"

_ENGLISH_REPLACEMENTS: tuple[tuple[str, str], ...] = (
    ("mildly", "слегка"),
    ("Mildly", "Слегка"),
    ("slightly", "слегка"),
    ("Slightly", "Слегка"),
    ("moderately", "умеренно"),
    ("Moderately", "Умеренно"),
    ("however", "однако"),
    ("However", "Однако"),
    ("probably", ""),
    ("Probably", ""),
)

# Латиница, которую оставляем: коды МКБ, единицы, редкие аббревиатуры бланка.
_LATIN_KEEP_RE = re.compile(
    r"^(?:[A-Z]\d{2}(?:\.\d+)?|mg|ml|EEG|ECG|CT|MRI|IQ)$",
    re.I,
)
_LATIN_WORD_RE = re.compile(r"\b[A-Za-z]{4,}\b")

_THERAPY_RE = re.compile(
    r"(инъекц|дозировк|мг/сут|кап/сут|мл\s*в/м|мл/сут|"
    r"коррекци\w*\s+терапи|"
    r"рисперидон|галоперидол|хлорпромазин|алимемазин|"
    r"левомепромазин|риперидон)",
    re.I,
)

_STRIP_THERAPY_PHRASES: tuple[re.Pattern[str], ...] = (
    re.compile(
        r"(?:,?\s*)?(?:в связи с (?:этим|возбуждением)[, ]*)?"
        r"(?:с седативной целью\s+)?"
        r"(?:была |было |выполнена |сделана )?"
        r"инъекц[^.]{0,180}",
        re.I,
    ),
    re.compile(
        r"(?:р-?ра?\.?|таб\.?|раствор)\s+[^,]{0,80}?"
        r"(?:\d+(?:[.,]\d+)?\s*%-?\s*\d+(?:[.,]\d+)?\s*мл(?:\s*в/м)?"
        r"|\d+(?:[.,]\d+)?\s*(?:мг|кап)/сут)",
        re.I,
    ),
    re.compile(r"дозировка[^.]{0,120}", re.I),
    re.compile(r"под контролем\s*АД,?\s*ЧСС", re.I),
    re.compile(
        r"проведена коррекция терапии:?\s*[^.]{0,160}",
        re.I,
    ),
)

_OCLIKI_RE = re.compile(r"на оклики", re.I)

_LABEL_RE = re.compile(
    r"^(?P<indent>\s*)(?P<bold>\*\*)?(?P<label>"
    r"Назначения|"
    r"План лечения \(дополнения к плану\)|"
    r"Психический статус(?: \(его изменение\))?"
    r")(?P<bold2>\*\*)?:\s*(?P<value>.*)$"
)

_SENTENCE_RE = re.compile(r"(?<=[.!?])\s+(?=[А-ЯЁA-Z«\"])")


def polish_diary(text: str) -> str:
    """Исправить типовые срывы модели в уже сгенерированном тексте."""
    if not text:
        return text
    out = fix_obvious_typos(text)
    out = _replace_english_leaks(out)
    out = _OCLIKI_RE.sub("на замечания", out)
    out = _reroute_therapy_fields(out)
    return out


def _replace_english_leaks(text: str) -> str:
    out = text
    for src, dst in _ENGLISH_REPLACEMENTS:
        if src in out:
            out = out.replace(src, dst)
    out = re.sub(r"\s{2,}", " ", out)

    def _drop_latin(match: re.Match[str]) -> str:
        word = match.group(0)
        if _LATIN_KEEP_RE.match(word):
            return word
        return ""

    cleaned = _LATIN_WORD_RE.sub(_drop_latin, out)
    return re.sub(r"[ \t]{2,}", " ", cleaned)


def _sentences(block: str) -> list[str]:
    block = block.strip()
    if not block:
        return []
    parts = _SENTENCE_RE.split(block)
    return [p.strip() for p in parts if p.strip()]


def _strip_therapy_phrases(sentence: str) -> str:
    out = sentence
    for pat in _STRIP_THERAPY_PHRASES:
        out = pat.sub("", out)
    out = re.sub(r"(?:,\s*){2,}", ", ", out)
    out = re.sub(r"\s+", " ", out)
    out = re.sub(r"\s+([.,;:])", r"\1", out)
    out = out.strip(" ,;")
    if out and out[-1] not in ".!?":
        out += "."
    return out


def _clinical_enough(text: str) -> bool:
    compact = re.sub(r"\s+", "", text)
    return len(compact) >= 24


def _join_plan(existing: str, moved: list[str]) -> str:
    chunks: list[str] = []
    base = existing.strip()
    if base and base.lower() not in {_PLAN_EMPTY, "нет", "-"}:
        chunks.append(base.rstrip("."))
    for item in moved:
        piece = item.strip().rstrip(".")
        if not piece:
            continue
        if any(piece.lower() in c.lower() or c.lower() in piece.lower() for c in chunks):
            continue
        chunks.append(piece)
    if not chunks:
        return _PLAN_EMPTY
    return "; ".join(chunks) + "."


def _reroute_status(value: str) -> tuple[str, list[str]]:
    moved: list[str] = []
    kept: list[str] = []
    for sent in _sentences(value):
        if not _THERAPY_RE.search(sent):
            kept.append(sent)
            continue
        moved.append(sent)
        cleaned = _strip_therapy_phrases(sent)
        if _clinical_enough(cleaned) and not _THERAPY_RE.search(cleaned):
            kept.append(cleaned)
        elif _clinical_enough(cleaned):
            # Остался препарат — не возвращаем в статус.
            pass
    return " ".join(kept).strip(), moved


def _rewrite_labeled(line: str, new_value: str) -> str:
    colon = line.find(":")
    if colon < 0:
        return line
    # Сохраняем «**Метка:**» / «Метка:» как было у модели.
    return line[: colon + 1] + " " + new_value


def _reroute_therapy_fields(text: str) -> str:
    lines = text.split("\n")
    moved: list[str] = []
    plan_idx: int | None = None

    for i, line in enumerate(lines):
        m = _LABEL_RE.match(line)
        if not m:
            continue
        label = m.group("label")
        value = m.group("value") or ""

        if label.startswith("Психический статус"):
            new_val, extra = _reroute_status(value)
            moved.extend(extra)
            if new_val:
                lines[i] = _rewrite_labeled(line, new_val)
        elif label == "Назначения":
            stripped = value.strip().rstrip(".")
            if stripped.lower() not in {_PRESCRIPTION_DEFAULT, ""}:
                moved.append(value.strip())
            lines[i] = _rewrite_labeled(line, _PRESCRIPTION_DEFAULT)
        elif label == "План лечения (дополнения к плану)":
            plan_idx = i

    if moved and plan_idx is not None:
        m = _LABEL_RE.match(lines[plan_idx])
        current = m.group("value") if m else _PLAN_EMPTY
        lines[plan_idx] = _rewrite_labeled(
            lines[plan_idx], _join_plan(current, moved),
        )
    elif moved and plan_idx is None:
        lines.append(
            "План лечения (дополнения к плану): " + _join_plan(_PLAN_EMPTY, moved)
        )

    return "\n".join(lines)
