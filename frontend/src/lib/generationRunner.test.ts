import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/errors";

const createPending = vi.fn();
const generate = vi.fn();
const patchRequest = vi.fn();
const fetchRequestDetail = vi.fn();

vi.mock("../api/endpoints", () => ({
  createPending: (...a: unknown[]) => createPending(...a),
  generate: (...a: unknown[]) => generate(...a),
  patchRequest: (...a: unknown[]) => patchRequest(...a),
  fetchRequestDetail: (...a: unknown[]) => fetchRequestDetail(...a),
}));

const { startBatchGeneration } = await import("./generationRunner");

const qc = { invalidateQueries: vi.fn() } as never;

function days(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    dayNumber: i + 1,
    isoDate: `2026-09-${String(i + 1).padStart(2, "0")}`,
    documentType: "daily",
    answers: {},
  }));
}

function llmDown() {
  return new ApiError("LLM_UNAVAILABLE", "LLM недоступна", 503);
}

async function runBatch(dayCount: number) {
  const parentId = await startBatchGeneration({
    qc,
    meta: {
      admission_date: "2026-09-01",
      date_from: "2026-09-01",
      date_to: `2026-09-0${dayCount}`,
      estimated_discharge: "",
      director_context: "",
      session_title: "тест",
    },
    narrativeAnswers: {},
    days: days(dayCount),
  });
  // Фоновая задача пакета: ждём, пока дойдёт до финального patch статуса.
  const finished = () =>
    patchRequest.mock.calls.some((c) => c[1]?.answers_anonymized !== undefined);
  for (let i = 0; i < 400 && !finished(); i++) {
    await new Promise((r) => setTimeout(r, 5));
  }
  return parentId;
}

describe("startBatchGeneration: устойчивость периода", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    let n = 0;
    createPending.mockImplementation(async () => ({ request_id: `child-${++n}` }));
    patchRequest.mockResolvedValue({});
    fetchRequestDetail.mockResolvedValue({ title_safe: "тест" });
  });

  it("продолжает период после одного сорвавшегося дня", async () => {
    generate.mockImplementation(async (req: { title_safe: string }) => {
      if (req.title_safe.startsWith("День 2 ")) throw llmDown();
      return {};
    });

    await runBatch(4);

    expect(generate).toHaveBeenCalledTimes(4);
    const final = patchRequest.mock.calls.at(-1)![1];
    // Остаётся «продолжаемым»: врач дозапустит только неудавшийся день.
    expect(final.status).toBe("pending");
    expect(final.title_safe).toMatch(/ошибок: 1/);
  });

  it("останавливается, если подряд сорвались два дня", async () => {
    generate.mockImplementation(async (req: { title_safe: string }) => {
      if (/^День [23] /.test(req.title_safe)) throw llmDown();
      return {};
    });

    await runBatch(5);

    expect(generate).toHaveBeenCalledTimes(3);
    expect(patchRequest.mock.calls.at(-1)![1].status).toBe("pending");
  });
});
