export const PROTOCOL_MAGIC = 0x47575244;
export const PROTOCOL_VERSION = 1;
export const HEADER_SIZE = 12;
export const MAX_PAYLOAD_SIZE = 32 * 1024 * 1024;
export const FRAME_META_SIZE = 32;
export const INPUT_EVENT_SIZE = 20;
export const MAX_FRAME_RAW_SIZE = 64 * 1024 * 1024;
export const MessageKind = {
    Hello: 1,
    Welcome: 2,
    Frame: 3,
    Input: 4,
    Ping: 5,
    Pong: 6,
    Error: 7,
    Bye: 8,
    ScreenInfo: 9,
};
export const HEADER_FLAG = {
    Deflate: 1 << 0,
};
export const PixelFormat = {
    BGRA8: 0,
};
export const FrameEncoding = {
    Raw: 0,
    Deflate: 1,
};
export const SERVER_VERSION = '0.1.0';
export const CLIENT_NAME_DEFAULT = 'ghostwire-cli';
//# sourceMappingURL=constants.js.map