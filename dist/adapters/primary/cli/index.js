import { createRequire } from 'node:module';
import { Command, Option } from 'commander';
import { GhostwireError, toGhostwireError } from '../../../core/domain/errors.js';
const require = createRequire(import.meta.url);
const pkg = require('../../../../package.json');
const HANDSHAKE_TIMEOUT_MS = 10_000;
const LOG_LEVELS = ['debug', 'info', 'warn', 'error'];
export function createCli(handlers, signal) {
    const program = new Command();
    program
        .name('ghostwire')
        .description('ghostwire — remote desktop over a custom RDP-like protocol (GWRD)')
        .version(pkg.version ?? '0.0.0')
        .showHelpAfterError();
    program
        .command('serve')
        .description('share this machine screen over TCP')
        .option('-a, --address <address>', 'listen address (host:port)', '0.0.0.0:5901')
        .addOption(new Option('-t, --token <token>', 'auth token').env('GHOSTWIRE_TOKEN').default(''))
        .option('--fps <n>', 'capture frame rate (1-60)', numberParser, 10)
        .option('--max-width <n>', 'max frame width in pixels, 0 = native', numberParser, 1600)
        .addOption(new Option('--compress <mode>', 'frame compression')
        .choices(['deflate', 'none'])
        .default('deflate'))
        .option('--tls-cert <path>', 'TLS certificate path (PEM, requires --tls-key)')
        .option('--tls-key <path>', 'TLS private key path (PEM, requires --tls-cert)')
        .option('--ping-interval <ms>', 'keepalive ping interval', numberParser, 5000)
        .option('--ping-timeout <ms>', 'keepalive timeout', numberParser, 15_000)
        .addOption(new Option('--log-level <level>', 'log verbosity')
        .choices(LOG_LEVELS)
        .default('info'))
        .action(async (flags) => {
        validateServeFlags(flags);
        const options = {
            address: flags.address,
            token: flags.token ?? '',
            fps: flags.fps,
            maxWidth: flags.maxWidth,
            compress: flags.compress,
            handshakeTimeoutMs: HANDSHAKE_TIMEOUT_MS,
            pingIntervalMs: flags.pingInterval,
            pingTimeoutMs: flags.pingTimeout,
            ...(flags.tlsCert && flags.tlsKey
                ? { tls: { certPath: flags.tlsCert, keyPath: flags.tlsKey } }
                : {}),
        };
        await handlers.serve({ options, logLevel: flags.logLevel }, signal);
    });
    program
        .command('connect')
        .description('view and control a remote ghostwire server in this terminal')
        .argument('<address>', 'server address (host:port)')
        .addOption(new Option('-t, --token <token>', 'auth token').env('GHOSTWIRE_TOKEN').default(''))
        .option('-n, --name <name>', 'client name shown to the server', 'ghostwire-cli')
        .option('--tls', 'use TLS for the connection')
        .option('--ca-cert <path>', 'custom CA certificate (PEM)')
        .option('--insecure', 'skip server certificate verification')
        .option('--ping-interval <ms>', 'keepalive ping interval', numberParser, 5000)
        .option('--ping-timeout <ms>', 'keepalive timeout', numberParser, 15_000)
        .addOption(new Option('--log-level <level>', 'log verbosity')
        .choices(LOG_LEVELS)
        .default('info'))
        .action(async (address, flags) => {
        const invocation = buildConnectInvocation(address, flags);
        await handlers.connect(invocation, signal);
    });
    program
        .command('info')
        .description('query a remote server and print its session info')
        .argument('<address>', 'server address (host:port)')
        .addOption(new Option('-t, --token <token>', 'auth token').env('GHOSTWIRE_TOKEN').default(''))
        .option('--tls', 'use TLS for the connection')
        .option('--ca-cert <path>', 'custom CA certificate (PEM)')
        .option('--insecure', 'skip server certificate verification')
        .option('--json', 'print raw JSON')
        .addOption(new Option('--log-level <level>', 'log verbosity')
        .choices(LOG_LEVELS)
        .default('info'))
        .action(async (address, flags) => {
        const invocation = buildConnectInvocation(address, {
            ...flags,
            name: 'ghostwire-info',
            pingInterval: 5000,
            pingTimeout: 15_000,
        });
        const info = await handlers.fetchInfo(invocation);
        if (flags.json) {
            process.stdout.write(`${JSON.stringify(info, null, 2)}\n`);
            return;
        }
        const lines = [
            `address: ${invocation.options.address}`,
            `screen:  ${info.screen.width}x${info.screen.height}`,
            `fps:     ${info.fps}`,
            `server:  ${info.serverVersion}`,
        ];
        process.stdout.write(`${lines.join('\n')}\n`);
    });
    return program;
}
export async function runCli(argv, handlers) {
    const controller = new AbortController();
    const program = createCli(handlers, controller.signal);
    let interrupts = 0;
    const onSignal = () => {
        interrupts += 1;
        if (interrupts > 1)
            process.exit(130);
        controller.abort();
    };
    process.on('SIGINT', onSignal);
    process.on('SIGTERM', onSignal);
    try {
        await program.parseAsync(argv);
        return 0;
    }
    catch (err) {
        const error = toGhostwireError(err);
        process.stderr.write(`error: ${error.message}\n`);
        return exitCodeFor(error);
    }
    finally {
        process.off('SIGINT', onSignal);
        process.off('SIGTERM', onSignal);
    }
}
function buildConnectInvocation(address, flags) {
    validateConnectFlags(address, flags);
    const options = {
        address,
        token: flags.token ?? '',
        clientName: flags.name,
        handshakeTimeoutMs: HANDSHAKE_TIMEOUT_MS,
        pingIntervalMs: flags.pingInterval,
        pingTimeoutMs: flags.pingTimeout,
        ...(flags.tls
            ? { tls: { ...(flags.caCert ? { caCertPath: flags.caCert } : {}), ...(flags.insecure ? { insecure: true } : {}) } }
            : {}),
    };
    return { options, logLevel: flags.logLevel };
}
function numberParser(value) {
    return Number(value);
}
function validateServeFlags(flags) {
    if (!flags.token) {
        throw new GhostwireError('config', 'serve: token is required (--token or GHOSTWIRE_TOKEN)');
    }
    requireInteger(flags.fps, 1, 60, 'serve: --fps');
    requireInteger(flags.maxWidth, 0, 16_384, 'serve: --max-width');
    requireInteger(flags.pingInterval, 100, 600_000, 'serve: --ping-interval');
    requireInteger(flags.pingTimeout, 100, 600_000, 'serve: --ping-timeout');
    if (flags.pingTimeout < flags.pingInterval) {
        throw new GhostwireError('config', 'serve: --ping-timeout must be >= --ping-interval');
    }
    if ((flags.tlsCert && !flags.tlsKey) || (!flags.tlsCert && flags.tlsKey)) {
        throw new GhostwireError('config', 'serve: --tls-cert and --tls-key must be used together');
    }
}
function validateConnectFlags(address, flags) {
    if (!address || !address.includes(':')) {
        throw new GhostwireError('config', 'connect: address must be host:port');
    }
    if (!flags.token) {
        throw new GhostwireError('config', 'connect: token is required (--token or GHOSTWIRE_TOKEN)');
    }
    if (!flags.name) {
        throw new GhostwireError('config', 'connect: --name must not be empty');
    }
    requireInteger(flags.pingInterval, 100, 600_000, 'connect: --ping-interval');
    requireInteger(flags.pingTimeout, 100, 600_000, 'connect: --ping-timeout');
    if (flags.pingTimeout < flags.pingInterval) {
        throw new GhostwireError('config', 'connect: --ping-timeout must be >= --ping-interval');
    }
    if (flags.caCert && !flags.tls) {
        throw new GhostwireError('config', 'connect: --ca-cert requires --tls');
    }
    if (flags.insecure && !flags.tls) {
        throw new GhostwireError('config', 'connect: --insecure requires --tls');
    }
}
function requireInteger(value, min, max, label) {
    if (!Number.isInteger(value) || value < min || value > max) {
        throw new GhostwireError('config', `${label} must be an integer between ${min} and ${max}`);
    }
}
function exitCodeFor(error) {
    switch (error.code) {
        case 'config':
            return 2;
        case 'auth':
            return 3;
        case 'timeout':
            return 4;
        case 'busy':
            return 5;
        default:
            return 1;
    }
}
//# sourceMappingURL=index.js.map