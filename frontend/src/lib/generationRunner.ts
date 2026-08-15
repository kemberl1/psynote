// Фоновые задачи генерации: запись уже в истории со статусом pending,
// пользователь может уйти на другой экран — задача доживёт до конца.
import type { QueryClient } from "@tanstack/react-query";
import { createPending, fetchRequestDetail, generate, patchRequest } from "../api/endpoints";
import type { Answers, GenerateRequest, HistoryChild, HistoryDetail } from "../api/types";
import { rebuildBatchDayJobs } from "./batchDiary";
import { formatDiaryDate } from "./format";
import {
    batchDayTitle,
    batchSessionTitle,
    displayHistoryTitle,
    isAutoBatchTitle,
    packBatchAnswers,
    pendingTitle,
    stripTitleStatus,
    withPendingSuffix,
    type BatchMeta,
} from "./historyTitles";

const running = new Set<string>();

export function isGenerationRunning(requestId: string): boolean {
  return running.has(requestId);
}

function invalidateAll(qc: QueryClient, requestId: string) {
  void qc.invalidateQueries({ queryKey: ["requests"] });
  void qc.invalidateQueries({ queryKey: ["requests", requestId] });
}

/** Одна дневниковая генерация: pending → generate(request_id) → done/failed. */
export async function startSingleGeneration(opts: {
  qc: QueryClient;
  documentType: string;
  answers: Answers;
  /** Регенерация существующей записи. */
  requestId?: string;
}): Promise<string> {
  let requestId = opts.requestId;
  if (requestId) {
    await patchRequest(requestId, {
      status: "pending",
      title_safe: pendingTitle(opts.documentType),
      answers_anonymized: opts.answers,
    });
    invalidateAll(opts.qc, requestId);
  } else {
    const pending = await createPending({
      document_type: opts.documentType,
      title_safe: pendingTitle(opts.documentType),
      answers_anonymized: opts.answers,
    });
    requestId = pending.request_id;
    invalidateAll(opts.qc, requestId);
  }

  if (running.has(requestId)) return requestId;
  running.add(requestId);

  const id = requestId;
  void (async () => {
    try {
      await generate({
        document_type: opts.documentType,
        answers: opts.answers,
        request_id: id,
      });
    } catch {
      try {
        await patchRequest(id, {
          status: "failed",
          title_safe: `${documentTypeShort(opts.documentType)} · Ошибка`,
        });
      } catch {
        /* ignore */
      }
    } finally {
      running.delete(id);
      invalidateAll(opts.qc, id);
    }
  })();

  return id;
}

function documentTypeShort(documentType: string): string {
  return pendingTitle(documentType).replace(" · Формируется…", "");
}

function customOrAutoTitle(meta: BatchMeta, dayCount: number, currentTitle?: string): string {
  const custom = meta.session_title?.trim();
  if (custom) return custom;
  if (currentTitle && !isAutoBatchTitle(currentTitle)) {
    return stripTitleStatus(currentTitle);
  }
  return batchSessionTitle(meta.date_from, meta.date_to, dayCount);
}

async function finalizeParentTitle(
  parentId: string,
  fallback: string,
  failed: number,
): Promise<string> {
  let base = fallback;
  try {
    const detail = await fetchRequestDetail(parentId);
    const current = stripTitleStatus(displayHistoryTitle(detail.title_safe));
    if (current) base = current;
  } catch {
    /* оставляем fallback */
  }
  if (failed > 0) return `${base} · ошибок: ${failed}`;
  return base;
}

export interface BatchDayJob {
  dayNumber: number;
  isoDate: string;
  documentType: string;
  answers: Answers;
}

/** Пакет: один parent pending + дети; по завершении parent → done. */
export async function startBatchGeneration(opts: {
  qc: QueryClient;
  meta: BatchMeta;
  narrativeAnswers: Answers;
  days: BatchDayJob[];
  replaceRequestId?: string;
}): Promise<string> {
  const dayCount = opts.days.length;
  const titlePending = withPendingSuffix(customOrAutoTitle(opts.meta, dayCount));
  const packed = packBatchAnswers(opts.narrativeAnswers, opts.meta);

  let parentId = opts.replaceRequestId;
  if (parentId) {
    await patchRequest(parentId, {
      status: "pending",
      title_safe: titlePending,
      answers_anonymized: packed,
    });
    invalidateAll(opts.qc, parentId);
  } else {
    const pending = await createPending({
      document_type: "batch",
      title_safe: titlePending,
      answers_anonymized: packed,
    });
    parentId = pending.request_id;
    invalidateAll(opts.qc, parentId);
  }

  if (running.has(parentId)) return parentId;
  running.add(parentId);

  const parent = parentId;
  void (async () => {
    let failed = 0;
    try {
      for (const day of opts.days) {
        const dayTitle = batchDayTitle(
          day.dayNumber,
          day.isoDate,
          day.documentType,
        );
        try {
          const childPending = await createPending({
            document_type: day.documentType,
            title_safe: `${dayTitle} · Формируется…`,
            parent_request_id: parent,
          });
          invalidateAll(opts.qc, parent);
          await generate({
            document_type: day.documentType,
            answers: day.answers,
            request_id: childPending.request_id,
            parent_request_id: parent,
            title_safe: dayTitle,
          } satisfies GenerateRequest);
        } catch {
          failed += 1;
        }
        invalidateAll(opts.qc, parent);
      }

      const fallback = customOrAutoTitle(opts.meta, dayCount);
      await patchRequest(parent, {
        status: failed === dayCount ? "failed" : "done",
        title_safe: await finalizeParentTitle(parent, fallback, failed),
        answers_anonymized: packed,
      });
    } catch {
      try {
        await patchRequest(parent, {
          status: "failed",
          title_safe: `${customOrAutoTitle(opts.meta, dayCount)} · Ошибка`,
        });
      } catch {
        /* ignore */
      }
    } finally {
      running.delete(parent);
      invalidateAll(opts.qc, parent);
    }
  })();

  return parentId;
}

function childMatchesDay(title: string, dayNumber: number, isoDate: string): boolean {
  return title.includes(`День ${dayNumber} · ${formatDiaryDate(isoDate)}`);
}

function childIsDone(child: HistoryChild): boolean {
  return child.status === "done" && Boolean(child.content);
}

/** Продолжить оборванный пакет: готовые дни не трогаем, pending/failed — заново. */
export async function resumeBatchGeneration(opts: {
  qc: QueryClient;
  detail: HistoryDetail;
}): Promise<string> {
  const rebuilt = rebuildBatchDayJobs(opts.detail.answers_anonymized ?? {});
  if (!rebuilt) {
    throw new Error("не удалось восстановить данные пакета");
  }

  const parent = opts.detail.request_id;
  const dayCount = rebuilt.days.length;
  const packed = packBatchAnswers(rebuilt.narrativeAnswers, rebuilt.meta);
  const children = opts.detail.children ?? [];

  await patchRequest(parent, {
    status: "pending",
    title_safe: withPendingSuffix(
      customOrAutoTitle(rebuilt.meta, dayCount, opts.detail.title_safe),
    ),
    answers_anonymized: packed,
  });
  invalidateAll(opts.qc, parent);

  if (running.has(parent)) return parent;
  running.add(parent);

  void (async () => {
    let failed = 0;
    try {
      for (const day of rebuilt.days) {
        const existing = children.find((c) =>
          childMatchesDay(c.title_safe, day.dayNumber, day.isoDate),
        );
        if (existing && childIsDone(existing)) continue;

        const dayTitle = batchDayTitle(day.dayNumber, day.isoDate, day.documentType);
        try {
          let childId = existing?.request_id;
          if (!childId) {
            const created = await createPending({
              document_type: day.documentType,
              title_safe: `${dayTitle} · Формируется…`,
              parent_request_id: parent,
            });
            childId = created.request_id;
          } else {
            await patchRequest(childId, {
              status: "pending",
              title_safe: `${dayTitle} · Формируется…`,
            });
          }
          invalidateAll(opts.qc, parent);
          await generate({
            document_type: day.documentType,
            answers: day.answers,
            request_id: childId,
            parent_request_id: parent,
            title_safe: dayTitle,
          } satisfies GenerateRequest);
        } catch {
          failed += 1;
        }
        invalidateAll(opts.qc, parent);
      }

      const fallback = customOrAutoTitle(
        rebuilt.meta,
        dayCount,
        opts.detail.title_safe,
      );
      await patchRequest(parent, {
        status: failed === dayCount ? "failed" : "done",
        title_safe: await finalizeParentTitle(parent, fallback, failed),
        answers_anonymized: packed,
      });
    } catch {
      try {
        await patchRequest(parent, {
          status: "failed",
          title_safe: `${customOrAutoTitle(rebuilt.meta, dayCount, opts.detail.title_safe)} · Ошибка`,
        });
      } catch {
        /* ignore */
      }
    } finally {
      running.delete(parent);
      invalidateAll(opts.qc, parent);
    }
  })();

  return parent;
}
