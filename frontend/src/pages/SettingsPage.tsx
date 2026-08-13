// Настройки аккаунта: подпись врача и заведующего для бланка МИС.
import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { friendlyError } from "../api/errors";
import { useAuth } from "../auth/AuthContext";
import { Banner, Button } from "../components/ui";
import {
  composeDailyDoctorLine,
  composeHeadSignature,
  doctorPositionLabel,
} from "../lib/exportSubstitutions";
import "../components/questionnaire/questionnaire.css";
import "./pages.css";
import "./settings.css";

const DOCTOR_CAPTION =
  "Фамилия, имя, отчество (при наличии) врача, должность, специальность, подпись";
const HEAD_CAPTION =
  "Фамилия, имя, отчество (при наличии) заведующего отделением, подпись";

export function SettingsPage() {
  const { doctor, updateProfile } = useAuth();
  const navigate = useNavigate();
  const [fullName, setFullName] = useState(doctor?.full_name ?? "");
  const [position, setPosition] = useState(doctor?.position ?? "");
  const [headName, setHeadName] = useState(doctor?.head_full_name ?? "");
  const [headPosition, setHeadPosition] = useState(doctor?.head_position ?? "");
  const [headInstitution, setHeadInstitution] = useState(
    doctor?.head_institution ?? "",
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    setFullName(doctor?.full_name ?? "");
    setPosition(doctor?.position ?? "");
    setHeadName(doctor?.head_full_name ?? "");
    setHeadPosition(doctor?.head_position ?? "");
    setHeadInstitution(doctor?.head_institution ?? "");
  }, [
    doctor?.full_name,
    doctor?.position,
    doctor?.head_full_name,
    doctor?.head_position,
    doctor?.head_institution,
  ]);

  useEffect(() => {
    if (!saved) return;
    const t = setTimeout(() => setSaved(false), 2600);
    return () => clearTimeout(t);
  }, [saved]);

  const previewDoctor = useMemo(
    () => ({
      full_name: fullName,
      position,
      head_full_name: headName,
      head_position: headPosition,
      head_institution: headInstitution,
    }),
    [fullName, position, headName, headPosition, headInstitution],
  );
  const dailyLine = composeDailyDoctorLine(previewDoctor);
  const tenDayDoctor = [fullName.trim(), doctorPositionLabel(position)].filter(Boolean).join(", ");
  const headLine = composeHeadSignature(previewDoctor);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSaving(true);
    try {
      await updateProfile({
        full_name: fullName.trim(),
        position: position.trim(),
        head_full_name: headName.trim(),
        head_position: headPosition.trim(),
        head_institution: headInstitution.trim(),
      });
      setSaved(true);
    } catch (err) {
      setError(friendlyError(err).detail || "Не удалось сохранить настройки");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <button className="back-link" onClick={() => navigate(-1)}>
        ← Назад
      </button>

      <header className="page-head">
        <h1 className="page-head__title">Настройки аккаунта</h1>
        <p className="page-head__subtitle">
          Эти данные подставляются в подпись дневника при экспорте в Word —
          не нужно вписывать их каждый раз вручную.
        </p>
      </header>

      {error && (
        <Banner tone="danger" title="Не сохранилось" text={error} />
      )}

      <form className="settings" onSubmit={(e) => void onSubmit(e)}>
        <section className="settings-card">
          <h2 className="settings-card__title">Ваши данные</h2>
          <p className="settings-card__hint">
            Попадут в графу «{DOCTOR_CAPTION}»
          </p>
          <label className="field">
            <span className="field__label">ФИО</span>
            <input
              className="field__input"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              placeholder="Иванов Иван Иванович"
              autoComplete="name"
              maxLength={240}
            />
          </label>
          <label className="field">
            <span className="field__label">Должность</span>
            <input
              className="field__input"
              value={position}
              onChange={(e) => setPosition(e.target.value)}
              placeholder="врач-психиатр детский"
              maxLength={240}
            />
          </label>
          <p className="settings-preview" aria-live="polite">
            <span className="settings-preview__label">Ежедневный осмотр</span>
            {dailyLine || "Лечащий врач, …"}
            <span className="settings-preview__label">Осмотр за 10 дней</span>
            {tenDayDoctor || "ФИО, должность"}
          </p>
        </section>

        <section className="settings-card">
          <h2 className="settings-card__title">Заведующий отделением</h2>
          <p className="settings-card__hint">
            Для осмотра за 10 дней. Попадут в графу «{HEAD_CAPTION}»
          </p>
          <label className="field">
            <span className="field__label">ФИО заведующего</span>
            <input
              className="field__input"
              value={headName}
              onChange={(e) => setHeadName(e.target.value)}
              placeholder="Иванов Иван Иванович"
              maxLength={240}
            />
          </label>
          <label className="field">
            <span className="field__label">Должность</span>
            <input
              className="field__input"
              value={headPosition}
              onChange={(e) => setHeadPosition(e.target.value)}
              placeholder="Врач-психиатр детский"
              maxLength={240}
            />
          </label>
          <label className="field">
            <span className="field__label">Лечебное учреждение</span>
            <textarea
              className="field__textarea"
              value={headInstitution}
              onChange={(e) => setHeadInstitution(e.target.value)}
              placeholder="ОПО№…, Общепсихиатрическое отделение для обслуживания детского населения №…"
              maxLength={400}
              rows={3}
            />
          </label>
          <p className="settings-preview" aria-live="polite">
            <span className="settings-preview__label">Как в бланке</span>
            {headLine || "ФИО. Должность (код). Учреждение"}
          </p>
        </section>

        <div className="settings__actions">
          <Button type="submit" variant="primary" loading={saving} disabled={saving}>
            Сохранить
          </Button>
          {saved && (
            <span className="settings__saved" role="status">
              Сохранено
            </span>
          )}
        </div>
      </form>
    </>
  );
}
