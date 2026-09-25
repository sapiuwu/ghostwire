export class GhostwireError extends Error {
    code;
    constructor(code, message, options) {
        super(message, options);
        this.name = 'GhostwireError';
        this.code = code;
    }
}
export function toGhostwireError(err, fallback = 'internal') {
    if (err instanceof GhostwireError)
        return err;
    if (err instanceof Error)
        return new GhostwireError(fallback, err.message, { cause: err });
    return new GhostwireError(fallback, String(err));
}
//# sourceMappingURL=errors.js.map