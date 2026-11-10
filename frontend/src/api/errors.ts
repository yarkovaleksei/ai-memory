// Go отдаёт ошибки кодом с параметрами (ADR-008), Wails кладёт их в поле cause.
// Здесь код превращается в ключ словаря: react-admin переводит message уведомлений сам.

export interface AppErrorCause {
  code: string;
  params?: Record<string, string> | null;
}

export class AppError extends Error {
  readonly code: string;
  readonly params: Record<string, string>;

  constructor(code: string, params: Record<string, string> = {}) {
    super(`aim.errors.${code}`);
    this.name = "AppError";
    this.code = code;
    this.params = params;
  }
}

const isCause = (v: unknown): v is AppErrorCause =>
  typeof v === "object" && v !== null && typeof (v as { code?: unknown }).code === "string";

export const toAppError = (err: unknown): AppError => {
  if (err instanceof AppError) return err;
  const cause = (err as { cause?: unknown } | null)?.cause;
  if (isCause(cause)) return new AppError(cause.code, cause.params ?? {});
  const detail = err instanceof Error ? err.message : String(err);
  return new AppError("unknown", { detail });
};

// call оборачивает вызов привязки: любая ошибка выходит наружу как AppError.
export const call = async <T>(fn: () => Promise<T>): Promise<T> => {
  try {
    return await fn();
  } catch (err) {
    throw toAppError(err);
  }
};
