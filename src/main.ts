#!/usr/bin/env node
import { runCli } from './adapters/primary/cli/index.js';
import type { CliHandlers } from './adapters/primary/cli/index.js';
import { TerminalViewer } from './adapters/primary/terminal/terminal-viewer.js';
import { GdiCapturer } from './adapters/secondary/capture/gdi-capturer.js';
import { DeflateCompressor } from './adapters/secondary/compress/deflate-compressor.js';
import { NullCompressor } from './adapters/secondary/compress/null-compressor.js';
import { SendInputInjector } from './adapters/secondary/inject/sendinput-injector.js';
import { ConsoleLogger } from './adapters/secondary/logging/console-logger.js';
import { TcpTransport } from './adapters/secondary/transport/tcp-transport.js';
import type { SessionInfo } from './core/domain/session/config.js';
import type { CompressorPort } from './core/ports/outbound/compress.js';
import { ClientService } from './core/services/client.service.js';
import { ServerService } from './core/services/server.service.js';

function compressorFor(mode: 'deflate' | 'none'): CompressorPort {
  return mode === 'none' ? new NullCompressor() : new DeflateCompressor();
}

const handlers: CliHandlers = {
  serve: async (invocation, signal) => {
    const logger = new ConsoleLogger(invocation.logLevel);
    const service = new ServerService({
      transport: new TcpTransport(),
      capturer: new GdiCapturer(),
      injector: new SendInputInjector(),
      compressor: compressorFor(invocation.options.compress),
      logger,
    });
    await service.serve(invocation.options, signal);
  },

  connect: async (invocation, signal) => {
    const logger = new ConsoleLogger(invocation.logLevel);
    const client = new ClientService({
      transport: new TcpTransport(),
      compressor: new DeflateCompressor(),
      logger,
    });
    const viewer = new TerminalViewer({
      viewer: client,
      logger,
      stdin: process.stdin,
      stdout: process.stdout,
    });
    await viewer.run(invocation.options, signal);
  },

  fetchInfo: async (invocation): Promise<SessionInfo> => {
    const logger = new ConsoleLogger(invocation.logLevel);
    const client = new ClientService({
      transport: new TcpTransport(),
      compressor: new DeflateCompressor(),
      logger,
    });
    const session = await client.connect(invocation.options);
    try {
      return session.info();
    } finally {
      await session.close('info');
    }
  },
};

process.exitCode = await runCli(process.argv, handlers);
