// Брифы дневников собираются в браузере (arcCompiler). Вкладка, открытая
// до деплоя, генерирует по старым правилам — так 16.09 пакет ушёл со старыми
// выходными и назначениями. Перед генерацией сверяем сборку с сервером.

export class StaleBuildError extends Error {
  constructor() {
    super("Вышла новая версия PsyNote — обновите страницу");
    this.name = "StaleBuildError";
  }
}

/** Бросает StaleBuildError, если на сервере уже другая сборка фронта. */
export async function assertFreshBuild(): Promise<void> {
  if (import.meta.env.DEV) return;
  let build: unknown;
  try {
    const res = await fetch(`/version.json?t=${Date.now()}`, { cache: "no-store" });
    if (!res.ok) return;
    build = ((await res.json()) as { build?: unknown }).build;
  } catch {
    // Сеть/старый сервер без version.json — не блокируем генерацию.
    return;
  }
  if (typeof build === "string" && build !== __BUILD_ID__) {
    throw new StaleBuildError();
  }
}
