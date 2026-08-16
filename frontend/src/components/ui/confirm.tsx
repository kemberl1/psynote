import {
    createContext,
    useCallback,
    useContext,
    useEffect,
    useId,
    useRef,
    useState,
    type ReactNode,
} from "react";
import { Button } from "./index";

export interface ConfirmOptions {
  title: string;
  text?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

export function useConfirm(): ConfirmFn {
  const fn = useContext(ConfirmContext);
  if (!fn) {
    throw new Error("useConfirm нужен внутри ConfirmProvider");
  }
  return fn;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  const resolver = useRef<((value: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFn>((next) => {
    resolver.current?.(false);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
      setOpts(next);
    });
  }, []);

  const close = useCallback((value: boolean) => {
    resolver.current?.(value);
    resolver.current = null;
    setOpts(null);
  }, []);

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {opts && (
        <ConfirmDialog
          {...opts}
          onCancel={() => close(false)}
          onConfirm={() => close(true)}
        />
      )}
    </ConfirmContext.Provider>
  );
}

function ConfirmDialog({
  title,
  text,
  confirmLabel = "ОК",
  cancelLabel = "Отмена",
  danger,
  onCancel,
  onConfirm,
}: ConfirmOptions & { onCancel: () => void; onConfirm: () => void }) {
  const titleId = useId();
  const textId = useId();
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const prev = document.activeElement;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancel();
        return;
      }
      if (e.key !== "Tab" || !panelRef.current) return;
      const focusable = panelRef.current.querySelectorAll<HTMLElement>("button");
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      if (prev instanceof HTMLElement) prev.focus();
    };
  }, [onCancel]);

  return (
    <div className="ui-confirm" role="presentation">
      <div
        className="ui-confirm__backdrop"
        onClick={onCancel}
        aria-hidden="true"
      />
      <div
        ref={panelRef}
        className="ui-confirm__panel"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={text ? textId : undefined}
      >
        <div className="ui-confirm__title" id={titleId}>
          {title}
        </div>
        {text && (
          <p className="ui-confirm__text" id={textId}>
            {text}
          </p>
        )}
        <div className="ui-confirm__actions">
          <Button autoFocus onClick={onCancel}>
            {cancelLabel}
          </Button>
          <Button
            variant={danger ? "danger" : "primary"}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
