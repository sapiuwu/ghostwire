import { readFileSync } from 'node:fs';
import net from 'node:net';
import tls from 'node:tls';
import { GhostwireError } from '../../../core/domain/errors.js';
const CONNECT_TIMEOUT_MS = 10_000;
const CLOSE_LINGER_MS = 2_000;
export function parseAddress(address) {
    const index = address.lastIndexOf(':');
    if (index < 0) {
        throw new GhostwireError('transport', `invalid address "${address}" (expected host:port)`);
    }
    let host = address.slice(0, index);
    if (host.startsWith('[') && host.endsWith(']'))
        host = host.slice(1, -1);
    const port = Number(address.slice(index + 1));
    if (!Number.isInteger(port) || port < 0 || port > 65535) {
        throw new GhostwireError('transport', `invalid port in address "${address}"`);
    }
    return { host, port };
}
let connSeq = 0;
class SocketConn {
    id;
    remoteAddress;
    socket;
    closing = false;
    constructor(socket) {
        this.socket = socket;
        this.id = `conn-${++connSeq}`;
        const host = socket.remoteAddress ?? 'unknown';
        const port = socket.remotePort ?? 0;
        this.remoteAddress = `${host}:${port}`;
        socket.setNoDelay(true);
        socket.on('error', () => {
            // consumers register their own handler; this prevents unhandled 'error' before they do
        });
    }
    write(chunk) {
        if (this.closing || this.socket.destroyed)
            return false;
        return this.socket.write(chunk);
    }
    onData(cb) {
        this.socket.on('data', cb);
    }
    onError(cb) {
        this.socket.on('error', cb);
    }
    onClose(cb) {
        this.socket.on('close', cb);
    }
    onDrain(cb) {
        this.socket.on('drain', cb);
    }
    close() {
        if (this.closing)
            return;
        this.closing = true;
        if (this.socket.destroyed)
            return;
        this.socket.end();
        const timer = setTimeout(() => {
            this.socket.destroy();
        }, CLOSE_LINGER_MS);
        timer.unref();
    }
}
class SocketListener {
    address;
    server;
    sockets = new Set();
    closed = false;
    constructor(server, address) {
        this.server = server;
        this.address = address;
        server.on('connection', (socket) => {
            this.sockets.add(socket);
            socket.on('close', () => this.sockets.delete(socket));
        });
    }
    onConnection(cb) {
        this.server.on('connection', (socket) => {
            cb(new SocketConn(socket));
        });
    }
    onError(cb) {
        this.server.on('error', cb);
    }
    async close() {
        if (this.closed)
            return;
        this.closed = true;
        for (const socket of this.sockets)
            socket.destroy();
        this.sockets.clear();
        await new Promise((resolve) => {
            this.server.close(() => resolve());
        });
    }
}
function formatAddress(host, port) {
    const display = host === '' ? '0.0.0.0' : host;
    return display.includes(':') ? `[${display}]:${port}` : `${display}:${port}`;
}
export class TcpTransport {
    async listen(address, tlsOptions) {
        const { host, port } = parseAddress(address);
        const server = tlsOptions ? this.createTlsServer(tlsOptions) : net.createServer();
        await new Promise((resolve, reject) => {
            const onError = (err) => {
                server.removeListener('listening', onListening);
                reject(new GhostwireError('transport', err.message, { cause: err }));
            };
            const onListening = () => {
                server.removeListener('error', onError);
                resolve();
            };
            server.once('error', onError);
            server.once('listening', onListening);
            server.listen(port, host === '' ? undefined : host);
        });
        const bound = server.address();
        const boundPort = typeof bound === 'object' && bound !== null ? bound.port : port;
        const boundHost = typeof bound === 'object' && bound !== null && bound.address ? bound.address : host;
        const listener = new SocketListener(server, formatAddress(boundHost, boundPort));
        return listener;
    }
    async dial(address, tlsOptions) {
        const { host, port } = parseAddress(address);
        const connectHost = host === '' ? '127.0.0.1' : host;
        const socket = await new Promise((resolve, reject) => {
            const opts = { host: connectHost, port };
            const stream = tlsOptions
                ? tls.connect(this.tlsConnectOptions(tlsOptions, connectHost, port))
                : net.connect(opts);
            const timer = setTimeout(() => {
                stream.destroy();
                reject(new GhostwireError('timeout', `connection to ${address} timed out`));
            }, CONNECT_TIMEOUT_MS);
            const onError = (err) => {
                clearTimeout(timer);
                stream.destroy();
                reject(new GhostwireError('transport', `failed to connect to ${address}: ${err.message}`, { cause: err }));
            };
            const onConnect = () => {
                clearTimeout(timer);
                stream.removeListener('error', onError);
                resolve(stream);
            };
            stream.once('error', onError);
            stream.once(tlsOptions ? 'secureConnect' : 'connect', onConnect);
        });
        return new SocketConn(socket);
    }
    createTlsServer(options) {
        let cert;
        let key;
        try {
            cert = readFileSync(options.certPath);
            key = readFileSync(options.keyPath);
        }
        catch (err) {
            throw new GhostwireError('transport', `failed to read TLS credentials: ${String(err)}`, { cause: err });
        }
        return tls.createServer({ cert, key });
    }
    tlsConnectOptions(options, host, port) {
        const opts = {
            host,
            port,
            rejectUnauthorized: options.insecure !== true,
        };
        if (!net.isIP(host))
            opts.servername = host;
        if (options.caCertPath) {
            try {
                opts.ca = readFileSync(options.caCertPath);
            }
            catch (err) {
                throw new GhostwireError('transport', `failed to read CA certificate: ${String(err)}`, { cause: err });
            }
        }
        return opts;
    }
}
//# sourceMappingURL=tcp-transport.js.map