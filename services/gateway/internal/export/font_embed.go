// Embedded Cyrillic TrueType fonts for PDF export.
//
// КИРИЛЛИЦА В PDF: стандартные «core»-шрифты PDF (Helvetica/Arial) НЕ содержат
// кириллических глифов — русский текст превратился бы в «кракозябры». Решение —
// встроить (go:embed) свободный TTF-шрифт с полным кириллическим покрытием и
// подключить его в fpdf через AddUTF8FontFromBytes (UTF-8 режим). Шрифт
// упаковывается прямо в бинарь, поэтому образ gateway самодостаточен и не
// зависит от системных шрифтов рантайма.
//
// Шрифт: DejaVu Sans (Condensed) — лицензия Bitstream Vera / DejaVu (свободная,
// разрешает встраивание, модификацию и коммерческое использование без роялти).
// Файлы взяты из дистрибутива github.com/go-pdf/fpdf (та же свободная лицензия).
package export

import _ "embed"

//go:embed fonts/DejaVuSansCondensed.ttf
var fontRegular []byte

//go:embed fonts/DejaVuSansCondensed-Bold.ttf
var fontBold []byte
