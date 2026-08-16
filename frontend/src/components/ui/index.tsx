// UI-примитивы PsyNote (дизайн-система docs/08 §2). Декомпозированы и
// типобезопасны; стили — в ui.css на дизайн-токенах.
import type { ButtonHTMLAttributes, ReactNode } from "react";
import "./ui.css";

// ─── Button ─────────────────────────────────────────────────────────────────
type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
type ButtonSize = "sm" | "md" | "lg";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  block?: boolean;
  loading?: boolean;
}

export function Button({
  variant = "secondary",
  size = "md",
  block = false,
  loading = false,
  disabled,
  children,
  className,
  ...rest
}: ButtonProps) {
  const classes = [
    "ui-btn",
    `ui-btn--${variant}`,
    size !== "md" ? `ui-btn--${size}` : "",
    block ? "ui-btn--block" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <button className={classes} disabled={disabled || loading} {...rest}>
      {loading && <Spinner />}
      {children}
    </button>
  );
}

// ─── Badge ──────────────────────────────────────────────────────────────────
interface BadgeProps {
  children: ReactNode;
  tone?: "default" | "accent" | "success";
  mono?: boolean;
}

export function Badge({ children, tone = "default", mono = false }: BadgeProps) {
  const classes = [
    "ui-badge",
    tone !== "default" ? `ui-badge--${tone}` : "",
    mono ? "ui-badge--mono" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return <span className={classes}>{children}</span>;
}

// ─── Spinner ──────────────────────────────────────────────────────────────────
export function Spinner({ size = "sm" }: { size?: "sm" | "lg" }) {
  return (
    <span
      className={`ui-spinner${size === "lg" ? " ui-spinner--lg" : ""}`}
      role="status"
      aria-label="Загрузка"
    />
  );
}

// ─── Skeleton ─────────────────────────────────────────────────────────────────
interface SkeletonProps {
  width?: string;
  height?: string;
  radius?: string;
}

export function Skeleton({ width = "100%", height = "14px", radius }: SkeletonProps) {
  return (
    <span
      className="ui-skeleton"
      style={{ width, height, borderRadius: radius }}
      aria-hidden="true"
    />
  );
}

// ─── EmptyState ─────────────────────────────────────────────────────────────
interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  text?: string;
  action?: ReactNode;
}

export function EmptyState({ icon, title, text, action }: EmptyStateProps) {
  return (
    <div className="ui-empty">
      {icon && <div className="ui-empty__icon">{icon}</div>}
      <div className="ui-empty__title">{title}</div>
      {text && <div className="ui-empty__text">{text}</div>}
      {action}
    </div>
  );
}

// ─── Banner (ошибки/приватность, docs/08 §7) ──────────────────────────────────
type BannerTone = "danger" | "warning" | "success";

interface BannerProps {
  tone?: BannerTone;
  icon?: ReactNode;
  title: string;
  text?: ReactNode;
  action?: ReactNode;
}

const DEFAULT_BANNER_ICON: Record<BannerTone, string> = {
  danger: "⚠",
  warning: "⚠",
  success: "✓",
};

export function Banner({
  tone = "danger",
  icon,
  title,
  text,
  action,
}: BannerProps) {
  return (
    <div className={`ui-banner ui-banner--${tone}`} role="alert">
      <div className="ui-banner__icon" aria-hidden="true">
        {icon ?? DEFAULT_BANNER_ICON[tone]}
      </div>
      <div className="ui-banner__body">
        <div className="ui-banner__title">{title}</div>
        {text && <div className="ui-banner__text">{text}</div>}
        {action && <div className="ui-banner__action">{action}</div>}
      </div>
    </div>
  );
}
