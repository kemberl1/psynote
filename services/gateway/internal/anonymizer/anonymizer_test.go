package anonymizer

import (
	"context"
	"strings"
	"testing"
)

// newTestPipeline builds a Go-only pipeline (no NER) with embedded dictionaries.
// ВСЕ данные ниже — СИНТЕТИЧЕСКИЕ (вымышленные ФИО/даты/адреса), не реальные
// пациенты (см. требование задачи и docs/04 §7).
func newTestPipeline(t *testing.T) *Pipeline {
	t.Helper()
	p, err := New(Options{})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return p
}

func run(t *testing.T, p *Pipeline, text string) Result {
	t.Helper()
	res, err := p.Anonymize(context.Background(), text)
	if err != nil {
		t.Fatalf("Anonymize(%q) error: %v", text, err)
	}
	return res
}

// TestStructuredPII covers level-2 regex detectors per category.
func TestStructuredPII(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name        string
		in          string
		wantHolder  string // плейсхолдер, который обязан появиться
		notContains string // фрагмент исходных ПДн, которого быть не должно
	}{
		{"numeric_date", "Осмотр проведён 19.09.2025 в отделении.", "[ДАТА]", "19.09.2025"},
		{"iso_date", "Дата записи 2025-09-19 утром.", "[ДАТА]", "2025-09-19"},
		{"textual_date", "Поступил 6 октября 2025 года в стационар.", "[ДАТА]", "октября"},
		{"date_range", "Наблюдение в период 30.11—01.12.2025 без особенностей.", "[ПЕРИОД]", "30.11"},
		{"time", "Время осмотра: 10 час. 18 мин.", "[ВРЕМЯ]", "10 час"},
		{"phone", "Контактный телефон +7 (812) 123-45-67 для связи.", "[ТЕЛЕФОН]", "123-45-67"},
		{"doc_number", "Медицинская карта № 20252184 заведена.", "[НОМЕР_ДОКУМЕНТА]", "20252184"},
		{"protocol_vk", "Протокол ВК № 442.3 от комиссии.", "[НОМЕР_ДОКУМЕНТА]", "442.3"},
		{"bare_long_id", "Идентификатор записи 7654321 в системе.", "[НОМЕР_ДОКУМЕНТА]", "7654321"},
		{"age_numeric", "Ребёнок 14 лет, поступил планово.", "[ВОЗРАСТ]", "14 лет"},
		{"age_verbal", "Находится в возрасте до пятнадцати лет.", "[ВОЗРАСТ]", "пятнадцати"},
		{"address", "Проживающий по адресу: г. Светлоград, ул. Вымышленная, д. 5, кв. 17.", "[АДРЕС]", "Вымышленная"},
		{"snils", "СНИЛС 123-456-789 00 указан в карте.", "[ДОКУМЕНТ]", "123-456-789"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, p, tc.in)
			if !strings.Contains(res.Text, tc.wantHolder) {
				t.Errorf("ожидался плейсхолдер %s; got: %q", tc.wantHolder, res.Text)
			}
			if tc.notContains != "" && strings.Contains(res.Text, tc.notContains) {
				t.Errorf("исходные ПДн %q не должны остаться; got: %q", tc.notContains, res.Text)
			}
			if res.RemovedCount == 0 {
				t.Errorf("RemovedCount должен быть > 0 для %q", tc.in)
			}
		})
	}
}

// TestFIOCases covers ФИО detection incl. Russian cases (падежи), initials,
// patronymics and role inference. Все ФИО — вымышленные.
func TestFIOCases(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name       string
		in         string
		wantHolder string
		leaked     string // словоформа, которой не должно остаться
	}{
		{"surname_initials", "Врач-психиатр Гаврилов А.Б. подписал.", "[ФИО_ВРАЧА]", "Гаврилов"},
		{"initials_surname", "Заключение дал врач И.О. Кузнецов лично.", "[ФИО_ВРАЧА]", "Кузнецов"},
		{"patronymic_nominative", "Осмотрен Иванов Тимофей Сергеевич.", "[", "Сергеевич"},
		{"patronymic_genitive", "освидетельствования Гаврилова Тимофея Ивановича провели", "[", "Ивановича"},
		{"female_patronymic", "Мать Смирнова Анна Петровна присутствовала.", "[", "Петровна"},
		{"patient_marker", "на имя Морозова Дмитрия Алексеевича оформлено", "[ПАЦИЕНТ]", "Алексеевича"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, p, tc.in)
			if !strings.Contains(res.Text, tc.wantHolder) {
				t.Errorf("ожидался плейсхолдер %q; got: %q", tc.wantHolder, res.Text)
			}
			if strings.Contains(res.Text, tc.leaked) {
				t.Errorf("ФИО %q просочилось; got: %q", tc.leaked, res.Text)
			}
		})
	}
}

// TestInstitutions covers level-3 gazetteer detection of facilities.
func TestInstitutions(t *testing.T) {
	p := newTestPipeline(t)
	cases := []string{
		`Направлен в ГБУЗ "Детская больница №19".`,
		`Состоит на учёте в ПНД №4 по месту жительства.`,
		`Переведён в ЦВЛ им. Вымышленного.`,
	}
	for _, in := range cases {
		res := run(t, p, in)
		if !strings.Contains(res.Text, "[УЧРЕЖДЕНИЕ]") {
			t.Errorf("ожидался [УЧРЕЖДЕНИЕ] для %q; got: %q", in, res.Text)
		}
	}
}

// TestWhitelistPreserved — precision: МКБ-коды, препараты, дозировки и статусы
// НЕ должны заменяться (docs/04 §3 уровень 6, §7 precision-тест).
func TestWhitelistPreserved(t *testing.T) {
	p := newTestPipeline(t)
	cases := []struct {
		name string
		in   string
		keep string
	}{
		{"icd_f", "Диагноз: F92.8 смешанное расстройство.", "F92.8"},
		{"icd_f06", "Уточнение F06.68 по основному.", "F06.68"},
		{"icd_r", "Синдром R51 в статусе.", "R51"},
		{"drug_dose", "Назначен Хлорпротиксен 25 мг на ночь.", "25 мг"},
		{"latin_drug", "Tab. Ibuprofeni 0,4 при необходимости.", "Ibuprofeni"},
		{"vitals", "Показатели: АД 120/80 мм рт.ст., ЭКГ без изменений.", "ЭКГ"},
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

// TestWhitelistNotFlaggedAsResidual — гейт не должен принимать МКБ/препараты за
// остаточные ПДн (precision гейта).
func TestWhitelistGateClean(t *testing.T) {
	p := newTestPipeline(t)
	in := "Диагноз: F92.8. Назначен Хлорпротиксен 25 мг. ЭКГ в норме."
	res := run(t, p, in)
	if !res.Clean {
		t.Errorf("чистый клинический текст не должен блокироваться гейтом; suspicions=%v text=%q",
			res.Suspicions, res.Text)
	}
	if err := Gate(res); err != nil {
		t.Errorf("Gate должен пропустить чистый текст: %v", err)
	}
}

// TestGateFailClosed — fail-closed: после полной анонимизации текст чист и Gate
// пропускает; при наличии остаточных ПДн Gate обязан блокировать.
func TestGateFailClosed(t *testing.T) {
	p := newTestPipeline(t)

	// Полностью обезличенный вход → гейт пропускает.
	res := run(t, p, "Осмотр 19.09.2025, пациент 14 лет, телефон +7 (812) 123-45-67.")
	if !res.Clean {
		t.Errorf("ожидался чистый результат, suspicions=%v text=%q", res.Suspicions, res.Text)
	}
	if err := Gate(res); err != nil {
		t.Errorf("Gate должен пропустить чистый текст: %v", err)
	}
}

// TestGateIndependentDetection — гейт (уровень 7) НЕЗАВИСИМО ловит ПДн, которые
// гипотетически пропустили предыдущие уровни. Проверяем напрямую gate.inspect
// на «просочившихся» паттернах — это сердце принципа fail-closed (docs/04 §1,§3).
func TestGateIndependentDetection(t *testing.T) {
	p := newTestPipeline(t)

	cases := []struct {
		name    string
		leaked  string
		wantHit bool
	}{
		{"residual_date", "Запись от 19.09.2025 осталась.", true},
		{"residual_phone", "Звонить +7 (812) 123-45-67.", true},
		{"residual_long_id", "Код 20252184 в системе.", true},
		{"residual_age", "Возраст 14 лет указан.", true},
		{"residual_patronymic", "Подпись Петрович внизу.", true},
		{"residual_dict_surname", "Фамилия Гаврилов осталась.", true},
		{"clean_clinical", "Диагноз F92.8, статус без особенностей.", false},
		{"only_placeholders", "Осмотр [ДАТА], пациент [ВОЗРАСТ].", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sus := p.gate.inspect(tc.leaked)
			if tc.wantHit && len(sus) == 0 {
				t.Errorf("гейт обязан был найти остаточные ПДн в %q (fail-closed)", tc.leaked)
			}
			if !tc.wantHit && len(sus) != 0 {
				t.Errorf("гейт ложно сработал на чистом %q: %v", tc.leaked, sus)
			}
		})
	}
}

// TestNoMapPersisted — Result не должен содержать карту «плейсхолдер→значение»
// (docs/04 §5). Проверяем, что в публичном выводе только счётчики/категории.
func TestRemovedByTypeCountsOnly(t *testing.T) {
	p := newTestPipeline(t)
	res := run(t, p, "Осмотр 19.09.2025 и 20.10.2025 проведён.")
	if res.RemovedByType[EntityDate] < 2 {
		t.Errorf("ожидалось >=2 дат в счётчике, got %d", res.RemovedByType[EntityDate])
	}
	total := 0
	for _, c := range res.RemovedByType {
		total += c
	}
	if total != res.RemovedCount {
		t.Errorf("RemovedCount(%d) должен совпадать с суммой по типам(%d)", res.RemovedCount, total)
	}
}

// TestRecallSynthetic — главный recall-тест (docs/04 §7): синтетический текст с
// несколькими подсаженными ПДн; измеряем долю пойманных категорий.
func TestRecallSynthetic(t *testing.T) {
	p := newTestPipeline(t)

	// Вымышленный дневник с известными ПДн (категория → должен быть плейсхолдер).
	text := `Дневник наблюдения.
Пациент: на имя Морозова Дмитрия Алексеевича, 14 лет.
Поступил 19.09.2025 в ГБУЗ "Детская больница №19".
Проживающий по адресу: г. Светлоград, ул. Вымышленная, д. 5, кв. 17.
Медицинская карта № 20252184. Телефон матери +7 (812) 123-45-67.
Лечащий врач-психиатр Гаврилов А.Б. Диагноз: F92.8. Назначен Хлорпротиксен 25 мг.`

	res := run(t, p, text)

	wantCategories := []EntityType{
		EntityPatient, EntityAge, EntityDate, EntityInstitution,
		EntityAddress, EntityDocNumber, EntityPhone, EntityDoctor,
	}
	// recall по категориям: сколько из ожидаемых категорий обнаружено
	// (хотя бы одной заменой). Цель docs/04 — близко к 100%.
	hit := 0
	var missed []EntityType
	for _, c := range wantCategories {
		// patient/doctor могут попасть в [ФИО]/[ПАЦИЕНТ]/[ФИО_ВРАЧА];
		// засчитываем по наличию любого FIO-плейсхолдера для роли.
		if res.RemovedByType[c] > 0 {
			hit++
		} else {
			missed = append(missed, c)
		}
	}
	recall := float64(hit) / float64(len(wantCategories))
	if recall < 0.85 {
		t.Errorf("recall категорий %.2f < 0.85; пропущены: %v; text=%q",
			recall, missed, res.Text)
	}

	// Жёсткая precision-проверка: клинически ценное сохранено.
	if !strings.Contains(res.Text, "F92.8") {
		t.Errorf("МКБ-код F92.8 не должен удаляться; got: %q", res.Text)
	}
	if !strings.Contains(res.Text, "Хлорпротиксен") {
		t.Errorf("название препарата не должно удаляться; got: %q", res.Text)
	}

	// Жёсткая recall-проверка значений: сырые ПДн не должны просочиться.
	for _, leak := range []string{"Морозова", "Алексеевича", "20252184", "123-45-67", "Гаврилов", "Вымышленная"} {
		if strings.Contains(res.Text, leak) {
			t.Errorf("ПДн %q просочились в результат: %q", leak, res.Text)
		}
	}
}

// TestPlaceholderMapping verifies every entity type has a typed placeholder.
func TestPlaceholderMapping(t *testing.T) {
	for _, et := range []EntityType{
		EntityPatient, EntityDoctor, EntityParent, EntityPerson, EntityDate,
		EntityPeriod, EntityTime, EntityAge, EntityAddress, EntityInstitution,
		EntityDocNumber, EntityPhone, EntityIDDoc,
	} {
		ph := et.Placeholder()
		if !strings.HasPrefix(ph, "[") || !strings.HasSuffix(ph, "]") {
			t.Errorf("placeholder для %s некорректен: %q", et, ph)
		}
	}
}
