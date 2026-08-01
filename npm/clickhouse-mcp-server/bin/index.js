#!/usr/bin/env node

const childProcess = require('child_process');

const goos = process.platform === 'win32' ? 'windows' : process.platform;
const goarch = process.arch === 'x64' ? 'amd64' : process.arch;
const suffix = process.platform === 'win32' ? '.exe' : '';
const packageName = `@futuretea/clickhouse-mcp-server-${goos}-${goarch}`;

let binaryPath;
try {
  binaryPath = require.resolve(`${packageName}/bin/clickhouse-mcp-server${suffix}`);
} catch {
  throw new Error(`Could not resolve binary path for platform/architecture: ${process.platform}/${process.arch}`);
}

const child = childProcess.spawn(binaryPath, ['mcp', ...process.argv.slice(2)], {
  stdio: 'inherit',
});

for (const signal of ['SIGTERM', 'SIGINT', 'SIGHUP']) {
  process.on(signal, () => {
    if (!child.killed) child.kill(signal);
  });
}

child.on('close', (code, signal) => {
  if (signal) process.exit(128 + (signal === 'SIGTERM' ? 15 : signal === 'SIGINT' ? 2 : 1));
  process.exit(code || 0);
});
