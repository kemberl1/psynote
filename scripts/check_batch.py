#!/usr/bin/env python3
"""Проверка сгенерированного периода по замечаниям врачей.

Вход — JSON ответа `GET /api/v1/history/{id}` для пакета (или массив таких
файлов). Скрипт печатает нарушения правил, из-за которых дневники
приходилось переписывать вручную:

  • бланк МИС: все строки на месте, ничего не склеено;
  • «Назначения» всегда «см. лист назначений», препараты — в план лечения;
  • за субботу и воскресенье дневников нет (кроме первых 3 суток и осмотра);
  • формула выходных — только в понедельник и с датами;
  • анамнез поступления и суицидальный анамнез не попадают в день;
  • ориентировка и критика не «скачут» между днями;
  • соседние дни не копируют друг друга.

Запуск:
    python3 scripts/check_batch.py batch.json [ещё.json ...]
    python3 scripts/check_batch.py -            # JSON из stdin
"""

from __future__ import annotations

import json
import re
import sys
from collections import Counter
from datetime import date

DAILY_LABELS = [
    "Жалобы",
    "Анамнез заболевания (дополнения к анамнезу)",
    "Анамнез жизни (дополнения к анамнезу)",
    "Физикальное исследование, локальный статус (его изменение)",
    "Психический статус",
    "Соматический статус",
    "Неврологический статус",
    "Диагноз",
    "Основное заболевание",
    "Сопутствующие заболевания",
    "Дополнительные сведения о заболевании",
    "Обоснование диагноза (при наличии дополнительных сведений)",
    "Назначения",
    "Выполнены медицинские вмешательства",
    "План обследования (дополнения к плану)",
    "План лечения (дополнения к плану)",
]

ADMISSION_RE = re.compile(
    r"интернат|\bдд[ие]\b|при[её]мн\w*\s+поко|сантранспорт|"
    r"направлен\w*\s+на\s+госпитализац|после выписки",
    re.I,
)
SUICIDE_HISTORY_RE = re.compile(
    r"попытк\w*\s+суицид|наносил\w*\s+порез|лезви|убить себя|"
    r"суицидальн\w*\s+мысл(?![^.]*отрица)",
    re.I,
)
DRUG_RE = re.compile(
    r"рисперидон|галоперидол|хлорпромазин|алимемазин|левомепромазин|"
    r"перициазин|сертралин|флуоксетин|эсциталопрам|вальпро",
    re.I,
)
ORIENT_RE = re.compile(r"Ориентирован[^.]*\.", re.I)
CRITICISM_RE = re.compile(r"Критик[а-яё]*[^.]*\.", re.I)
WEEKEND_RE = re.compile(r"за период выходных дней", re.I)
TITLE_DATE_RE = re.compile(r"День (\d+) · (\d{2})\.(\d{2})\.(\d{4})")


def _day_info(title: str) -> tuple[int, date] | None:
    m = TITLE_DATE_RE.search(title or "")
    if not m:
        return None
    return int(m.group(1)), date(int(m.group(4)), int(m.group(3)), int(m.group(2)))


def _norm_orientation(text: str) -> str:
    m = ORIENT_RE.search(text)
    if not m:
        return "нет"
    low = m.group(0).lower()
    if re.search(r"затрудн|с ошибкой|не назыв|частично|не ориентир", low):
        return "неполная"
    return "верно"


def _norm_criticism(text: str) -> str:
    m = CRITICISM_RE.search(text)
    if not m:
        return "нет"
    low = m.group(0).lower()
    for key in ("отсутств", "не выявл", "формиру", "формальн", "снижен",
                "частично", "достаточн", "сохран"):
        if key in low:
            return key
    return "иное"


BOILERPLATE_RE = re.compile(
    r"^(?:Сознание|Ориентирован|Зрительный контакт|В беседе с врачом|Дистанцию|"
    r"Мимика|Мышление|Интеллект|Критика|Волевой контроль|Психопродуктивн|"
    r"Эмоциональные реакции|Настроение|Фон настроения|Речь|Обращённую речь|"
    r"Опасных тенденций|Суицидн)",
    re.I,
)


def _status_sentences(text: str) -> list[str]:
    """Только психический статус: повтор шаблонных строк бланка — не копипаст."""
    line = next(
        (ln for ln in text.split("\n") if ln.replace("**", "").startswith("Психический статус")),
        "",
    )
    value = line.split(":", 1)[1] if ":" in line else ""
    out = []
    for sentence in re.split(r"(?<=[.!?])\s+", value):
        sentence = sentence.strip()
        if len(sentence) > 60 and not BOILERPLATE_RE.match(sentence):
            out.append(sentence)
    return out


def check_batch(batch: dict) -> list[str]:
    problems: list[str] = []
    children = batch.get("children") or []
    if not children:
        return ["в пакете нет дневников"]

    orientations: Counter[str] = Counter()
    criticisms: Counter[str] = Counter()
    seen_sentences: dict[str, str] = {}

    for child in children:
        title = child.get("title_safe") or "?"
        text = child.get("content") or ""
        short = title[:28]
        if child.get("status") != "done" or not text:
            problems.append(f"{short}: день не сгенерирован ({child.get('status')})")
            continue

        is_exam = "Осмотр за 10 дней" in title
        lines = text.split("\n")
        starts = [ln.split(":", 1)[0].strip().replace("**", "") for ln in lines]
        if not is_exam:
            for label in DAILY_LABELS:
                if label not in starts:
                    problems.append(f"{short}: нет строки бланка «{label}»")
            for ln in lines:
                inner = [x for x in DAILY_LABELS if f" {x}:" in ln]
                if inner:
                    problems.append(f"{short}: строки склеены — «{inner[0]}» внутри строки")

        for ln in lines:
            if ln.startswith("Назначения:") and ln.strip() != "Назначения: см. лист назначений":
                problems.append(f"{short}: «Назначения» не по форме → {ln.strip()[:60]}")
            if ln.startswith("Психический статус") and DRUG_RE.search(ln):
                problems.append(f"{short}: препарат в психическом статусе")

        if ADMISSION_RE.search(text):
            problems.append(f"{short}: анамнез поступления в дневнике дня")
        if SUICIDE_HISTORY_RE.search(text):
            problems.append(f"{short}: суицидальный анамнез как событие дня")

        info = _day_info(title)
        if info:
            day_no, day = info
            weekend = day.weekday() >= 5
            if weekend and day_no > 3 and not is_exam:
                problems.append(f"{short}: дневник за выходной после 3-го дня")
            has_formula = bool(WEEKEND_RE.search(text))
            if has_formula and day.weekday() != 0:
                problems.append(f"{short}: формула выходных не в понедельник")
            if has_formula and "[ВЫХОДНЫЕ]" not in text and not re.search(
                r"выходных дней с \d", text
            ):
                problems.append(f"{short}: в формуле выходных нет дат")

        if not is_exam:
            orientations[_norm_orientation(text)] += 1
            criticisms[_norm_criticism(text)] += 1
            if _norm_criticism(text) == "нет":
                problems.append(f"{short}: в статусе нет критики")

        for sentence in _status_sentences(text):
            if sentence in seen_sentences and seen_sentences[sentence] != title:
                problems.append(
                    f"{short}: наблюдение дословно повторяет "
                    f"«{seen_sentences[sentence][:16]}» → {sentence[:60]}…"
                )
            seen_sentences.setdefault(sentence, title)

    if len(orientations) > 1:
        problems.append(f"ориентировка «скачет» по дням: {dict(orientations)}")
    if len([k for k in criticisms if k != "нет"]) > 2:
        problems.append(f"критика «скачет» по дням: {dict(criticisms)}")
    return problems


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(__doc__)
        return 2
    total = 0
    for path in argv[1:]:
        raw = sys.stdin.read() if path == "-" else open(path, encoding="utf-8").read()
        data = json.loads(raw)
        batch = data.get("data", data)
        name = batch.get("title_safe") or path
        problems = check_batch(batch)
        total += len(problems)
        print(f"\n=== {name}: дней {len(batch.get('children') or [])}, "
              f"замечаний {len(problems)}")
        for p in problems:
            print(f"  • {p}")
    print(f"\nвсего замечаний: {total}")
    return 1 if total else 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
