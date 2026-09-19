import { afterEach, describe, expect, it, vi } from "vitest";
import { StaleBuildError, assertFreshBuild } from "./buildVersion";

function serverBuild(build: string | null, ok = true) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({ ok, json: async () => ({ build }) })),
  );
}

describe("assertFreshBuild", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
  });

  it("пропускает, если сборка на сервере та же", async () => {
    vi.stubEnv("DEV", false);
    serverBuild(__BUILD_ID__);
    await expect(assertFreshBuild()).resolves.toBeUndefined();
  });

  it("останавливает устаревшую вкладку", async () => {
    vi.stubEnv("DEV", false);
    serverBuild("другая-сборка");
    await expect(assertFreshBuild()).rejects.toBeInstanceOf(StaleBuildError);
  });

  it("не блокирует генерацию, если version.json недоступен", async () => {
    vi.stubEnv("DEV", false);
    serverBuild(null, false);
    await expect(assertFreshBuild()).resolves.toBeUndefined();
    vi.stubGlobal("fetch", vi.fn(async () => { throw new TypeError("offline"); }));
    await expect(assertFreshBuild()).resolves.toBeUndefined();
  });
});
