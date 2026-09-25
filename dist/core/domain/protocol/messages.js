import { GhostwireError } from '../errors.js';
const encoder = new TextEncoder();
const decoder = new TextDecoder();
function parseObject(payload) {
    let value;
    try {
        value = JSON.parse(decoder.decode(payload));
    }
    catch (err) {
        throw new GhostwireError('protocol', 'invalid JSON payload', { cause: err });
    }
    if (typeof value !== 'object' || value === null || Array.isArray(value)) {
        throw new GhostwireError('protocol', 'payload must be a JSON object');
    }
    return value;
}
function requireNumber(obj, key) {
    const value = obj[key];
    if (typeof value !== 'number' || !Number.isFinite(value)) {
        throw new GhostwireError('protocol', `missing or invalid field "${key}"`);
    }
    return value;
}
function requireString(obj, key, maxLength) {
    const value = obj[key];
    if (typeof value !== 'string') {
        throw new GhostwireError('protocol', `missing or invalid field "${key}"`);
    }
    if (value.length > maxLength) {
        throw new GhostwireError('protocol', `field "${key}" exceeds ${maxLength} characters`);
    }
    return value;
}
function optionalString(obj, key, maxLength) {
    const value = obj[key];
    if (value === undefined || value === null)
        return undefined;
    if (typeof value !== 'string') {
        throw new GhostwireError('protocol', `invalid field "${key}"`);
    }
    if (value.length > maxLength) {
        throw new GhostwireError('protocol', `field "${key}" exceeds ${maxLength} characters`);
    }
    return value;
}
function parseScreen(obj) {
    const screen = obj['screen'];
    if (typeof screen !== 'object' || screen === null) {
        throw new GhostwireError('protocol', 'missing or invalid field "screen"');
    }
    const s = screen;
    const width = requireNumber(s, 'width');
    const height = requireNumber(s, 'height');
    if (width <= 0 || height <= 0 || width > 65535 || height > 65535) {
        throw new GhostwireError('protocol', 'screen dimensions out of range');
    }
    return { width: Math.trunc(width), height: Math.trunc(height) };
}
export function encodeHello(payload) {
    return encoder.encode(JSON.stringify({
        protocolVersion: payload.protocolVersion,
        clientName: payload.clientName,
        token: payload.token,
    }));
}
export function decodeHello(payload) {
    const obj = parseObject(payload);
    const protocolVersion = requireNumber(obj, 'protocolVersion');
    if (!Number.isInteger(protocolVersion) || protocolVersion < 0 || protocolVersion > 255) {
        throw new GhostwireError('protocol', 'invalid protocolVersion');
    }
    return {
        protocolVersion,
        clientName: requireString(obj, 'clientName', 64),
        token: requireString(obj, 'token', 512),
    };
}
export function encodeWelcome(payload) {
    const out = { ok: payload.ok };
    if (payload.reason !== undefined)
        out['reason'] = payload.reason;
    if (payload.serverVersion !== undefined)
        out['serverVersion'] = payload.serverVersion;
    if (payload.screen !== undefined)
        out['screen'] = payload.screen;
    if (payload.fps !== undefined)
        out['fps'] = payload.fps;
    return encoder.encode(JSON.stringify(out));
}
export function decodeWelcome(payload) {
    const obj = parseObject(payload);
    const ok = obj['ok'];
    if (typeof ok !== 'boolean') {
        throw new GhostwireError('protocol', 'missing or invalid field "ok"');
    }
    const reason = optionalString(obj, 'reason', 256);
    if (!ok) {
        return reason === undefined ? { ok } : { ok, reason };
    }
    const serverVersion = optionalString(obj, 'serverVersion', 32);
    const fps = obj['fps'] === undefined ? undefined : requireNumber(obj, 'fps');
    const out = { ok, screen: parseScreen(obj) };
    if (reason !== undefined)
        out.reason = reason;
    if (serverVersion !== undefined)
        out.serverVersion = serverVersion;
    if (fps !== undefined)
        out.fps = fps;
    return out;
}
export function encodePing(payload) {
    return encoder.encode(JSON.stringify({ t: payload.t }));
}
export function decodePing(payload) {
    const obj = parseObject(payload);
    return { t: requireNumber(obj, 't') };
}
export function encodeError(payload) {
    return encoder.encode(JSON.stringify({ code: payload.code, message: payload.message }));
}
export function decodeError(payload) {
    const obj = parseObject(payload);
    return {
        code: requireString(obj, 'code', 64),
        message: requireString(obj, 'message', 512),
    };
}
export function encodeBye(payload) {
    return encoder.encode(JSON.stringify({ reason: payload.reason }));
}
export function decodeBye(payload) {
    const obj = parseObject(payload);
    return { reason: requireString(obj, 'reason', 256) };
}
export function encodeScreenInfo(payload) {
    return encoder.encode(JSON.stringify({ width: payload.width, height: payload.height }));
}
export function decodeScreenInfo(payload) {
    const obj = parseObject(payload);
    const width = requireNumber(obj, 'width');
    const height = requireNumber(obj, 'height');
    if (width <= 0 || height <= 0 || width > 65535 || height > 65535) {
        throw new GhostwireError('protocol', 'screen dimensions out of range');
    }
    return { width: Math.trunc(width), height: Math.trunc(height) };
}
//# sourceMappingURL=messages.js.map