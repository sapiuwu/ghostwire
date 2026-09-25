export type ErrorCode =
  | 'protocol'
  | 'version'
  | 'auth'
  | 'busy'
  | 'timeout'
  | 'closed'
  | 'transport'
  | 'unsupported'
  | 'config'
  | 'internal';

export class GhostwireError extends Error {
  readonly code: ErrorCode;

  constructor(code: ErrorCode, message: string, options?: { cause?: unknown }) {
    super(message, options);
    this.name = 'GhostwireError';
    this.code = code;
  }
}

export function toGhostwireError(err: unknown, fallback: ErrorCode = 'internal'): GhostwireError {
  if (err instanceof GhostwireError) return err;
  if (err instanceof Error) return new GhostwireError(fallback, err.message, { cause: err });
  return new GhostwireError(fallback, String(err));
}
