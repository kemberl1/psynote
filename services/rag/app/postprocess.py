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
    r"левомепромазин|риперидон|перициазин|неволептом)",
    re.I,
)

_CORRECTION_RE = re.compile(
    r"инъекц|коррекци\w*\s+терапи|дозировк\w*\s+(увеличен|снижен)|"
    r"увеличен[ао]?\s+до|снижен[ао]?\s+до|отмен[еёа]|добавлен",
    re.I,
)

_DRUG_STEMS = (
    "перициазин", "неволептом", "рисперидон", "риперидон", "галоперидол",
    "хлорпромазин", "алимемазин", "левомепромазин", "кветиапин", "арипипразол",
    "хлорпротиксен", "хлопротиксен", "тиоридазин", "оланзапин", "клозапин",
    "сульпирид", "зуклопентиксол", "карбамазепин", "вальпроат", "конвулекс",
    "депакин", "бипериден", "циклодол", "диазепам", "феназепам", "гидроксизин",
    "атаракс", "тералиджен", "сонапакс", "сероквель", "азалептин", "клопиксол",
    "флуоксетин", "сертралин", "атомоксетин", "метилфенидат",
    "тригексифенидил", "клоназепам",
)

_DRUG_FIND_RE = re.compile(
    r"(?<![а-яёa-z])(" + "|".join(_DRUG_STEMS) + r")[а-яё]*",
    re.I,
)

# «Физическое удержание» — запрещённая формулировка. «Мягкая фиксация» —
# штатная формула отделения из корпуса, её не вырезаем.
_ILLEGAL_HOLD_RE = re.compile(
    r"(?:в такие моменты\s+)?(?:требуется\s+)?"
    r"физическ\w*\s+удержани[еяю](?:\s+и\s+помощь\s+персонала)?|"
    r"удержани[еяю]\s+(?:реб[её]нк|пациент|персонал)|"
    r"иммобилиз\w*",
    re.I,
)
_LEGAL_FIXATION = (
    "применена мягкая фиксация конечностей под контролем медперсонала"
)

_STAFF_HOLD_RE = re.compile(
    r"удержива\w{0,12}\s+с\s+помощью\s+персонала",
    re.I,
)

_STAFF_HELP_SENT_RE = re.compile(
    r"[^.!?\n]*помощ\w*\s+персонал[^.!?\n]*[.!?]?",
    re.I,
)

_HYGIENE_RE = re.compile(
    r"одев|мыт|гигиен|самообслуж|ест |корм|переодев|туалет|ложк",
    re.I,
)

_ADMISSION_PLOT_RE = re.compile(
    r"[^.!?\n]*("
    r"направлен\w*\s+на\s+госпитализац|"
    r"пакет документов|"
    r"интернат|"
    r"\bдд[ие]\b|"
    r"при[её]мн\w*\s+поко|"
    r"сантранспорт|"
    r"доставлен\w*.{0,48}при[её]мн|"
    r"после выписки|"
    r"мать забрала|"
    r"рекомендованн\w+\s+лечени\w+\s+принимал"
    r")[^.!?\n]*[.!?]?",
    re.I,
)

# Плейсхолдеры анонимайзера, которых не должно быть в готовом дневнике.
# [ДАТА], [ФИО_ВРАЧА], [НОМЕР_ИБ] — штатные, их не трогаем.
_LEAK_PLACEHOLDER_RE = re.compile(
    r"\[(?:УЧРЕЖДЕНИЕ|НОМЕР_ДОКУМЕНТА|ПАЦИЕНТ|АДРЕС|ТЕЛЕФОН)\]",
)

_HOLD_ATTEMPT_RE = re.compile(r"при попытке удержани\w*", re.I)
_HOLD_BY_HAND_RE = re.compile(r"при удержании за руку", re.I)

_FULLY_CALMED_RE = re.compile(
    r"после фиксации успокоил\w*(?!\s+непродолжительн)",
    re.I,
)
_STILL_AGITATED_RE = re.compile(
    r"не успокоил|успокоил\w*\s+непродолжительн|"
    r"вновь\s+(?:становил\w*|стал\w*)\s+возбудим|"
    r"затем\s+вновь",
    re.I,
)
_INJECTION_SENT_RE = re.compile(
    r"[^.!?\n]*инъекц[^.!?\n]*[.!?]?",
    re.I,
)

_WEEKEND_FORMULA_RE = re.compile(
    r"за период выходных дней[^.!?\n]*дежурн\w*\s+мед\s+персонал[^.!?\n]*[.!?]?",
    re.I,
)
_HYGIENE_DUMP_RE = re.compile(
    r"[^.!?\n]*одевает[^.!?\n]{0,80}гигиеническ[^.!?\n]*[.!?]?",
    re.I,
)

_ANAMNESIS_LABELS = (
    "Анамнез заболевания (дополнения к анамнезу)",
    "Анамнез жизни (дополнения к анамнезу)",
)
_ADDITIONAL_LABEL = "Дополнительные сведения о заболевании"
_STATUS_LABEL_PREFIX = "Психический статус"
_EPICRISIS_LABEL = "Этапный эпикриз"

_DRUG_REDACT = "[препарат другого пациента — не копировать]"

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


def flatten_answer_text(value: object) -> str:
    """Собрать весь свободный текст ответов для белого списка препаратов."""
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        return "\n".join(flatten_answer_text(v) for v in value.values())
    if isinstance(value, list):
        return "\n".join(flatten_answer_text(v) for v in value)
    return ""


def mentioned_drugs(text: str) -> set[str]:
    """Стемы препаратов, которые реально названы во входе этого пациента."""
    if not text:
        return set()
    return {m.group(1).lower() for m in _DRUG_FIND_RE.finditer(text)}


def sanitize_corpus_sample(text: str) -> str:
    """Few-shot: стиль статуса без чужих лекарств и анамнеза поступления.

    «Мягкая фиксация» из корпуса оставляем — это рабочая формула отделения.
    """
    if not text:
        return text
    out = _ADMISSION_PLOT_RE.sub(" ", text)
    out = _LEAK_PLACEHOLDER_RE.sub("", out)
    out = _normalize_restraint_language(out)
    out = _DRUG_FIND_RE.sub(_DRUG_REDACT, out)
    out = re.sub(r"[ \t]{2,}", " ", out)
    out = re.sub(r"\n{3,}", "\n\n", out)
    return out.strip()


def polish_diary(
    text: str,
    *,
    allowed_drugs: set[str] | None = None,
    lock_anamnesis: bool = False,
    lock_additional_none: bool = False,
) -> str:
    """Исправить типовые срывы модели в уже сгенерированном тексте."""
    if not text:
        return text
    out = fix_obvious_typos(text)
    out = _replace_english_leaks(out)
    out = _OCLIKI_RE.sub("на замечания", out)
    out = _normalize_restraint_language(out)
    out = _LEAK_PLACEHOLDER_RE.sub("", out)
    out = _scrub_history_sections(
        out,
        lock_anamnesis=lock_anamnesis,
        lock_additional_none=lock_additional_none,
    )
    out = _reroute_therapy_fields(out)
    out = _drop_injection_if_calmed(out)
    if allowed_drugs is not None:
        out = _strip_unmentioned_drugs(out, allowed_drugs)
        out = _collapse_empty_plan(out)
    out = re.sub(r"[ \t]{2,}", " ", out)
    return out


def _normalize_restraint_language(text: str) -> str:
    """«Физическое удержание» → формула корпуса; мягкую фиксацию не трогать."""
    out = _ILLEGAL_HOLD_RE.sub(_LEGAL_FIXATION, text)
    out = _HOLD_ATTEMPT_RE.sub("при попытке остановить", out)
    out = _HOLD_BY_HAND_RE.sub("если взять за руку", out)
    out = _STAFF_HOLD_RE.sub("удерживается", out)

    def _staff_help(match: re.Match[str]) -> str:
        sent = match.group(0)
        if _HYGIENE_RE.search(sent) or re.search(r"фиксац", sent, re.I):
            return sent
        if re.search(
            r"физическ\w*\s+удерж|требует\w*\s+помощ",
            sent,
            re.I,
        ):
            return " "
        return sent

    out = _STAFF_HELP_SENT_RE.sub(_staff_help, out)
    out = re.sub(r"[ \t]{2,}", " ", out)
    out = re.sub(r" +\n", "\n", out)
    out = re.sub(r"\n{3,}", "\n\n", out)
    return out


def _parse_labeled(line: str) -> tuple[str | None, str]:
    m = re.match(r"^\s*(?:\*\*)?(.+?)(?:\*\*)?:\s*(.*)$", line)
    if not m:
        return None, ""
    return m.group(1).strip(), m.group(2)


def _scrub_additional_value(value: str, *, lock_none: bool) -> str:
    if lock_none:
        return "нет"
    weekend = _WEEKEND_FORMULA_RE.search(value)
    rest = _WEEKEND_FORMULA_RE.sub(" ", value)
    rest = _ADMISSION_PLOT_RE.sub(" ", rest)
    rest = _HYGIENE_DUMP_RE.sub(" ", rest)
    rest = _LEAK_PLACEHOLDER_RE.sub("", rest)
    rest = re.sub(r"\s+", " ", rest).strip(" ;,")
    parts: list[str] = []
    if weekend:
        parts.append(weekend.group(0).strip().rstrip("."))
    for sent in _sentences(rest):
        if not _clinical_enough(sent):
            continue
        if _HYGIENE_RE.search(sent) and len(sent) < 140:
            continue
        parts.append(sent.rstrip("."))
        if len(parts) >= 3:
            break
    if not parts:
        return "нет"
    return ". ".join(parts) + "."


def _scrub_history_sections(
    text: str,
    *,
    lock_anamnesis: bool,
    lock_additional_none: bool,
) -> str:
    """Интернат/приёмный покой не должны жить в анамнезе, статусе и доп. сведениях."""
    lines = text.split("\n")
    for i, line in enumerate(lines):
        label, value = _parse_labeled(line)
        if not label:
            continue
        if label in _ANAMNESIS_LABELS:
            if lock_anamnesis:
                lines[i] = _rewrite_labeled(line, "без дополнений")
                continue
            cleaned = _ADMISSION_PLOT_RE.sub(" ", value)
            cleaned = _LEAK_PLACEHOLDER_RE.sub("", cleaned)
            cleaned = re.sub(r"\s+", " ", cleaned).strip(" ;,.")
            lines[i] = _rewrite_labeled(line, cleaned or "без дополнений")
        elif label == _ADDITIONAL_LABEL:
            lines[i] = _rewrite_labeled(
                line,
                _scrub_additional_value(
                    value, lock_none=lock_additional_none,
                ),
            )
        elif label.startswith(_STATUS_LABEL_PREFIX) or label == _EPICRISIS_LABEL:
            cleaned = _ADMISSION_PLOT_RE.sub(" ", value)
            cleaned = _LEAK_PLACEHOLDER_RE.sub("", cleaned)
            cleaned = re.sub(r"\s+", " ", cleaned).strip()
            if cleaned:
                lines[i] = _rewrite_labeled(line, cleaned)
    return "\n".join(lines)


def _drop_injection_if_calmed(text: str) -> str:
    """После фиксации успокоился полностью — инъекцию на тот же эпизод не оставляем."""
    lines = text.split("\n")
    status = ""
    plan_idx: int | None = None
    for i, line in enumerate(lines):
        label, value = _parse_labeled(line)
        if not label:
            continue
        if label.startswith(_STATUS_LABEL_PREFIX):
            status = value
        elif label == "План лечения (дополнения к плану)":
            plan_idx = i
    if plan_idx is None or not status:
        return text
    if _STILL_AGITATED_RE.search(status):
        return text
    if not _FULLY_CALMED_RE.search(status):
        return text
    plan_line = lines[plan_idx]
    _, plan_val = _parse_labeled(plan_line)
    cleaned = _INJECTION_SENT_RE.sub(" ", plan_val)
    cleaned = re.sub(r"\s+", " ", cleaned).strip(" ;,.")
    if not cleaned or not _CORRECTION_RE.search(cleaned):
        lines[plan_idx] = _rewrite_labeled(plan_line, _PLAN_EMPTY)
    else:
        if cleaned[-1] not in ".!?":
            cleaned += "."
        lines[plan_idx] = _rewrite_labeled(plan_line, cleaned)
    return "\n".join(lines)


def _drug_clause_re(stem: str) -> re.Pattern[str]:
    return re.compile(
        r"(?:(?:р-?ра?|таб|капс|раствор(?:а|ом)?)\.?\s+)?"
        + re.escape(stem)
        + r"[а-яё]*"
        r"(?:\s+\d+(?:[.,]\d+)?\s*%(?:\s*[-—]?\s*\d+(?:[.,]\d+)?\s*(?:мл|мг))?)?"
        r"(?:\s+по\s+[\d\-—/]+(?:\s*кап(?:ли|\.)?)?)?"
        r"(?:\s+\d+(?:[.,]\d+)?\s*(?:мг|кап|мл)(?:/сут)?)?",
        re.I,
    )


def _strip_unmentioned_drugs(text: str, allowed: set[str]) -> str:
    allowed_l = {a.lower() for a in allowed}
    found = {m.group(1).lower() for m in _DRUG_FIND_RE.finditer(text)}
    foreign = sorted(found - allowed_l)
    if not foreign:
        return text
    out = text
    for stem in foreign:
        out = _drug_clause_re(stem).sub("", out)
    out = re.sub(r"(?:,\s*){2,}", ", ", out)
    out = re.sub(r"[ \t]{2,}", " ", out)
    out = re.sub(r" ,", ",", out)
    return out


def _collapse_empty_plan(text: str) -> str:
    lines = text.split("\n")
    for i, line in enumerate(lines):
        m = _LABEL_RE.match(line)
        if not m or m.group("label") != "План лечения (дополнения к плану)":
            continue
        val = (m.group("value") or "").strip(" ;,.")
        if len(val) < 8 or val.lower() in {_PLAN_EMPTY, "нет", "-", ""}:
            lines[i] = _rewrite_labeled(line, _PLAN_EMPTY)
    return "\n".join(lines)


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
                if _CORRECTION_RE.search(stripped):
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
            "План лечения (дополнения к плану): " +
            _join_plan(_PLAN_EMPTY, moved)
        )

    return "\n".join(lines)
