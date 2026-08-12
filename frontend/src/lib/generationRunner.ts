// Фоновые задачи генерации: запись уже в истории со статусом pending,
// пользователь может уйти на другой экран — задача доживёт до конца.
import type { QueryClient } from "@tanstack/react-query";
import { createPending, generate, patchRequest } from "../api/endpoints";
import type { Answers, GenerateRequest } from "../api/types";
import {
  batchDayTitle,
  batchDoneTitle,
  batchPendingTitle,
  packBatchAnswers,
  pendingTitle,
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
  const titlePending = batchPendingTitle(
    opts.meta.date_from,
    opts.meta.date_to,
    dayCount,
  );
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

      const doneTitle = batchDoneTitle(
        opts.meta.date_from,
        opts.meta.date_to,
        dayCount,
      );
      await patchRequest(parent, {
        status: failed === dayCount ? "failed" : "done",
        title_safe:
          failed > 0 ? `${doneTitle} · ошибок: ${failed}` : doneTitle,
        answers_anonymized: packed,
      });
    } catch {
      try {
        await patchRequest(parent, {
          status: "failed",
          title_safe: `${batchDoneTitle(opts.meta.date_from, opts.meta.date_to, dayCount)} · Ошибка`,
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
