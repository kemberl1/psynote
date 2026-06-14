package anonymizer

import (
	"context"
	"strings"
	"testing"
)

// ЭТАП 3.1 — регресс-тесты усиления детекторов остаточной ПДн.
// ВСЕ данные синтетические (вымышленные ФИО/даты/номера), реальные пациенты НЕ
// используются (docs/04 §7). Цель — поднять recall по residual_patronymic /
// residual_date / residual_long_id, не ослабляя precision Этапа 2.1.

// TestPatronymicAllCases — отчества во ВСЕХ падежах (м.р. и ж.р.) должны
// вырезаться. Главная причина блокировок (residual_patronymic) — косвенные
// падежи женских отчеств, которые раньше не ловились (суффикс кодировался как
// «овна», а склонения «овны/овне/овну/овной» — нет).
func TestPatronymicAllCases(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name   string
		in     string
		leaked string // словоформа, которой не должно остаться
	}{
		// Мужские отчества во всех падежах.
		{"male_nom", "Обратился Иванович повторно сегодня.", "Иванович"},
		{"male_gen", "Жалоба от Ивановича поступила вчера.", "Ивановича"},
		{"male_dat", "Передано Ивановичу на рассмотрение.", "Ивановичу"},
		{"male_ins", "Беседа проведена Ивановичем лично.", "Ивановичем"},
		{"male_pre", "Сведения об Алексеевиче занесены.", "Алексеевиче"},
		{"male_short_ich", "Осмотрен Кузьмич утром.", "Кузьмич"},
		// Женские отчества во всех падежах — КЛЮЧЕВОЙ фикс residual_patronymic.
		{"fem_nom", "Присутствовала Петровна на приёме.", "Петровна"},
		{"fem_gen", "Со слов Петровны выяснено следующее.", "Петровны"},
		{"fem_dat", "Передано Петровне для ознакомления.", "Петровне"},
		{"fem_acc", "Пригласили Петровну в кабинет.", "Петровну"},
		{"fem_ins", "Подписано Петровной собственноручно.", "Петровной"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, p, tc.in)
			if strings.Contains(res.Text, tc.leaked) {
				t.Errorf("отчество %q просочилось; got: %q", tc.leaked, res.Text)
			}
			if !strings.Contains(res.Text, "[") {
				t.Errorf("ожидался ФИО-плейсхолдер для %q; got: %q", tc.in, res.Text)
			}
		})
	}
}

// TestFullNameObliqueWithPatronymic — полное ФИО с отчеством в косвенном падеже
// вырезается ЦЕЛИКОМ (фамилия+имя+отчество), а не только отчество.
func TestFullNameObliqueWithPatronymic(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name   string
		in     string
		leaks  []string
		holder string
	}{
		{
			// Роль может определиться как [ФИО] (маркер «освидетельствования»
			// содержит кириллическое окончание, которое ASCII-only \w в RE2 не
			// добирает) — для recall важно, что ФИО вырезано ЦЕЛИКОМ.
			"genitive_full",
			"освидетельствования Гаврилова Тимофея Ивановича провели",
			[]string{"Гаврилова", "Тимофея", "Ивановича"},
			"[",
		},
		{
			"dative_full",
			"направлено Смирновой Анне Петровне для сведения",
			[]string{"Смирновой", "Анне", "Петровне"},
			"[",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, p, tc.in)
			for _, leak := range tc.leaks {
				if strings.Contains(res.Text, leak) {
					t.Errorf("часть ФИО %q просочилась; got: %q", leak, res.Text)
				}
			}
			if !strings.Contains(res.Text, tc.holder) {
				t.Errorf("ожидался плейсхолдер %q; got: %q", tc.holder, res.Text)
			}
		})
	}
}

// TestPatronymicLikeCommonWordsKept — precision: нарицательные слова, совпадающие
// с отчественным паттерном (-ич), с заглавной буквы НЕ должны вырезаться и НЕ
// должны ложно блокироваться гейтом.
func TestPatronymicLikeCommonWordsKept(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		in   string
		keep string
	}{
		{"Паралич конечностей не выявлен при осмотре.", "Паралич"},
		{"Москвич по происхождению, адаптирован.", "Москвич"},
	}
	for _, tc := range cases {
		res := run(t, p, tc.in)
		if !strings.Contains(res.Text, tc.keep) {
			t.Errorf("нарицательное %q не должно вырезаться; got: %q", tc.keep, res.Text)
		}
		if !res.Clean {
			t.Errorf("чистый текст %q не должен блокироваться гейтом; suspicions=%v", tc.in, res.Suspicions)
		}
	}
}

// TestRareDateFormats — редкие форматы дат (residual_date) должны вырезаться.
func TestRareDateFormats(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name   string
		in     string
		leaked string
	}{
		// Без ведущих нулей и с 2-значным годом.
		{"no_leading_zeros", "Осмотр 9.9.25 в кабинете.", "9.9.25"},
		{"slash_short_year", "Запись 1/3/24 утром.", "1/3/24"},
		// «дд месяца» прописью БЕЗ года (родительный падеж месяца).
		{"day_month_no_year", "Поступил 6 октября в стационар.", "6 октября"},
		{"day_month_no_year2", "Выписан 1 декабря домой.", "1 декабря"},
		// Год с сокращением «г.»/«гг.».
		{"year_g", "Заключение датировано 2025 г. без правок.", "2025 г."},
		{"years_range_gg", "Наблюдался в 2023-2024 гг. амбулаторно.", "2023-2024 гг."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, p, tc.in)
			if strings.Contains(res.Text, tc.leaked) {
				t.Errorf("дата %q просочилась; got: %q", tc.leaked, res.Text)
			}
			if !strings.Contains(res.Text, "[ДАТА]") && !strings.Contains(res.Text, "[ПЕРИОД]") {
				t.Errorf("ожидался [ДАТА]/[ПЕРИОД] для %q; got: %q", tc.in, res.Text)
			}
		})
	}
}

// TestClinicalNumbersKept — precision (Этап 2.1): клинические числа, диапазоны,
// дозы и показатели НЕ должны приниматься за даты/идентификаторы.
func TestClinicalNumbersKept(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name string
		in   string
		keep string
	}{
		{"bp_ratio", "АД 123/70 мм рт.ст. в норме.", "123/70"},
		{"range_dose", "Курс 18-21 день по схеме.", "18-21"},
		{"qtc", "QTc=0.40 по ЭКГ без удлинения.", "0.40"},
		{"dose_mg", "Назначен препарат 25 мг на ночь.", "25 мг"},
		{"dose_big_mg", "Суточно 1200 мг в три приёма.", "1200 мг"},
		{"icd", "Диагноз F92.8 подтверждён.", "F92.8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, p, tc.in)
			if !strings.Contains(res.Text, tc.keep) {
				t.Errorf("клинически ценное %q должно сохраниться; got: %q", tc.keep, res.Text)
			}
		})
	}
}

// TestLongIDByMarker — длинный/маркированный числовой идентификатор вырезается;
// доза/показатель — нет.
func TestLongIDByMarker(t *testing.T) {
	p := newTestPipeline(t)

	// Позитив: ID после знака «№» (даже без слова-маркера из закрытого списка).
	pos := run(t, p, "Направление № 12345 оформлено.")
	if strings.Contains(pos.Text, "12345") {
		t.Errorf("номер по «№» должен вырезаться; got: %q", pos.Text)
	}
	if !strings.Contains(pos.Text, "[НОМЕР_ДОКУМЕНТА]") {
		t.Errorf("ожидался [НОМЕР_ДОКУМЕНТА]; got: %q", pos.Text)
	}

	// Позитив: голый длинный ID (>=7 цифр).
	pos2 := run(t, p, "Идентификатор 98765432 в реестре.")
	if strings.Contains(pos2.Text, "98765432") {
		t.Errorf("длинный ID должен вырезаться; got: %q", pos2.Text)
	}

	// Негатив: доза «1200 мг» и показатель НЕ должны трактоваться как ID.
	neg := run(t, p, "Доза 1200 мг, показатель 320 в пределах нормы.")
	if !strings.Contains(neg.Text, "1200 мг") {
		t.Errorf("доза не должна вырезаться как ID; got: %q", neg.Text)
	}
	if !strings.Contains(neg.Text, "320") {
		t.Errorf("короткий показатель не должен вырезаться как ID; got: %q", neg.Text)
	}
}

// TestUnblockedDocument — КЛЮЧЕВОЙ интеграционный тест. Синтетический «документ»,
// имитирующий ранее заблокированный кейс (отчество в косвенном падеже + редкая
// дата + год «г.»), после Anonymize должен давать Clean=true: усиленные
// детекторы дочищают именно то, на что раньше срабатывал гейт (fail-closed).
func TestUnblockedDocument(t *testing.T) {
	p := newTestPipeline(t)

	doc := `Дневник наблюдения.
Со слов Петровны состояние стабильное.
освидетельствования Гаврилова Тимофея Ивановича провели 6 октября.
Заключение датировано 2025 г. Направление № 12345.
Диагноз: F92.8. Назначен Хлорпротиксен 25 мг.`

	res, err := p.Anonymize(context.Background(), doc)
	if err != nil {
		t.Fatalf("Anonymize error: %v", err)
	}

	if !res.Clean {
		t.Fatalf("документ должен проходить гейт (Clean=true) после дочистки; suspicions=%v\ntext=%q",
			res.Suspicions, res.Text)
	}
	if err := Gate(res); err != nil {
		t.Errorf("Gate должен пропустить дочищенный документ: %v", err)
	}

	// ПДн вычищены.
	for _, leak := range []string{"Петровны", "Ивановича", "Гаврилова", "Тимофея", "6 октября", "2025 г.", "12345"} {
		if strings.Contains(res.Text, leak) {
			t.Errorf("остаточная ПДн %q просочилась; got: %q", leak, res.Text)
		}
	}
	// Клиника сохранена (precision).
	if !strings.Contains(res.Text, "F92.8") || !strings.Contains(res.Text, "Хлорпротиксен") {
		t.Errorf("клинические данные не должны удаляться; got: %q", res.Text)
	}
}

// TestUnblockedDocumentGateBefore — контроль причинности: тот же остаточный
// фрагмент, поданный «как есть» (без анонимизации) на gate.inspect, ВСЁ ещё
// детектируется гейтом. Это доказывает, что гейт НЕ ослаблен: он по-прежнему
// ловит непойманное отчество в косвенном падеже и редкую дату.
func TestGateStillCatchesResidual(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name   string
		leaked string
	}{
		{"residual_fem_patronymic_oblique", "Подпись Петровны внизу."},
		{"residual_male_patronymic_oblique", "Жалоба Ивановича записана."},
		{"residual_date", "Запись от 9.9.25 осталась."},
		// Голый длинный ID (>=7 цифр) — ровно то, что ловит residual_long_id
		// гейта (\d{7,}). Маркированный «№ N» дочищает ДЕТЕКТОР до гейта, поэтому
		// здесь проверяем именно непойманный голый ID.
		{"residual_long_id", "Код 98765432 в системе."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sus := p.gate.inspect(tc.leaked)
			if len(sus) == 0 {
				t.Errorf("гейт обязан найти остаточную ПДн в %q (fail-closed не ослаблен)", tc.leaked)
			}
		})
	}
}
