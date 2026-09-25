import { describe, expect, it } from 'vitest';
import { GhostwireError } from '../src/core/domain/errors.js';
import {
  MessageDecoder,
  decodeControlPayload,
  encodeJSONMessage,
  encodeMessage,
} from '../src/core/domain/protocol/codec.js';
import { MessageKind, PROTOCOL_VERSION } from '../src/core/domain/protocol/constants.js';
import {
  decodeBye,
  decodeError,
  decodeHello,
  decodePing,
  decodeScreenInfo,
  decodeWelcome,
  encodeBye,
  encodeError,
  encodeHello,
  encodePing,
  encodeScreenInfo,
  encodeWelcome,
} from '../src/core/domain/protocol/messages.js';
import { decodeFrameMeta, encodeFrameMeta } from '../src/core/domain/protocol/frame.js';
import { FrameEncoding, PixelFormat } from '../src/core/domain/protocol/constants.js';

describe('message codec', () => {
  it('roundtrips a message through a single feed', () => {
    const payload = Buffer.from('hello ghostwire');
    const encoded = encodeMessage(MessageKind.Ping, payload);
    const decoder = new MessageDecoder();
    const messages = decoder.feed(encoded);
    expect(messages).toHaveLength(1);
    expect(messages[0]!.kind).toBe(MessageKind.Ping);
    expect(Buffer.from(messages[0]!.payload).toString()).toBe('hello ghostwire');
  });

  it('reassembles messages split byte by byte', () => {
    const first = encodeMessage(MessageKind.Hello, Buffer.from('one'));
    const second = encodeMessage(MessageKind.Bye, Buffer.from('two'));
    const stream = Buffer.concat([first, second]);
    const decoder = new MessageDecoder();
    const collected: number[] = [];
    for (const byte of stream) {
      for (const message of decoder.feed(Buffer.from([byte]))) {
        collected.push(message.kind);
        expect(Buffer.from(message.payload).toString()).toMatch(/^(one|two)$/);
      }
    }
    expect(collected).toEqual([MessageKind.Hello, MessageKind.Bye]);
  });

  it('rejects a stream with bad magic', () => {
    const decoder = new MessageDecoder();
    expect(() => decoder.feed(Buffer.alloc(12))).toThrowError(/out of sync/);
  });

  it('rejects an unsupported protocol version', () => {
    const encoded = encodeMessage(MessageKind.Ping, Buffer.alloc(0));
    encoded.writeUInt8(99, 4);
    const decoder = new MessageDecoder();
    expect(() => decoder.feed(encoded)).toThrowError(/protocol version 99/);
  });

  it('compresses large control payloads transparently', () => {
    const value = { reason: 'x'.repeat(4096) };
    const encoded = encodeJSONMessage(MessageKind.Bye, value);
    const decoder = new MessageDecoder();
    const [message] = decoder.feed(encoded);
    expect(message!.flags & 1).toBe(1);
    expect(decodeBye(decodeControlPayload(message!)).reason).toBe(value.reason);
  });
});

describe('control messages', () => {
  it('roundtrips hello', () => {
    const payload = encodeHello({ protocolVersion: PROTOCOL_VERSION, clientName: 'cli', token: 'secret' });
    expect(decodeHello(payload)).toEqual({
      protocolVersion: PROTOCOL_VERSION,
      clientName: 'cli',
      token: 'secret',
    });
  });

  it('rejects hello with invalid token type', () => {
    const payload = new TextEncoder().encode(JSON.stringify({ protocolVersion: 1, clientName: 'x', token: 5 }));
    expect(() => decodeHello(payload)).toThrowError(GhostwireError);
  });

  it('roundtrips welcome', () => {
    const payload = encodeWelcome({
      ok: true,
      serverVersion: '0.1.0',
      screen: { width: 1920, height: 1080 },
      fps: 30,
    });
    expect(decodeWelcome(payload)).toEqual({
      ok: true,
      serverVersion: '0.1.0',
      screen: { width: 1920, height: 1080 },
      fps: 30,
    });
  });

  it('roundtrips rejected welcome', () => {
    const payload = encodeWelcome({ ok: false, reason: 'unauthorized' });
    expect(decodeWelcome(payload)).toEqual({ ok: false, reason: 'unauthorized' });
  });

  it('rejects welcome with bad screen', () => {
    const payload = new TextEncoder().encode(
      JSON.stringify({ ok: true, screen: { width: 0, height: 1080 } }),
    );
    expect(() => decodeWelcome(payload)).toThrowError(/screen dimensions/);
  });

  it('roundtrips ping, error, bye, screen info', () => {
    expect(decodePing(encodePing({ t: 1234567890 }))).toEqual({ t: 1234567890 });
    expect(decodeError(encodeError({ code: 'busy', message: 'nope' }))).toEqual({
      code: 'busy',
      message: 'nope',
    });
    expect(decodeBye(encodeBye({ reason: 'done' }))).toEqual({ reason: 'done' });
    expect(decodeScreenInfo(encodeScreenInfo({ width: 800, height: 600 }))).toEqual({
      width: 800,
      height: 600,
    });
  });

  it('rejects non-object payloads', () => {
    const payload = new TextEncoder().encode('[1,2,3]');
    expect(() => decodePing(payload)).toThrowError(/JSON object/);
  });
});

describe('frame meta', () => {
  it('roundtrips', () => {
    const meta = {
      seq: 42,
      timestampMs: 1_700_000_000_123,
      x: 0,
      y: 0,
      width: 1920,
      height: 1080,
      pixelFormat: PixelFormat.BGRA8,
      encoding: FrameEncoding.Deflate,
      rawLength: 1920 * 1080 * 4,
    };
    expect(decodeFrameMeta(encodeFrameMeta(meta))).toEqual(meta);
  });

  it('rejects unsupported pixel format', () => {
    const encoded = encodeFrameMeta({
      seq: 1,
      timestampMs: 1,
      x: 0,
      y: 0,
      width: 2,
      height: 2,
      pixelFormat: 7,
      encoding: FrameEncoding.Raw,
      rawLength: 16,
    });
    expect(() => decodeFrameMeta(encoded)).toThrowError(/pixel format/);
  });

  it('rejects raw length mismatch', () => {
    const encoded = encodeFrameMeta({
      seq: 1,
      timestampMs: 1,
      x: 0,
      y: 0,
      width: 2,
      height: 2,
      pixelFormat: PixelFormat.BGRA8,
      encoding: FrameEncoding.Raw,
      rawLength: 15,
    });
    expect(() => decodeFrameMeta(encoded)).toThrowError(/raw length/);
  });

  it('rejects truncated header', () => {
    expect(() => decodeFrameMeta(new Uint8Array(10))).toThrowError(/truncated/);
  });
});
