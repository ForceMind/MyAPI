const net = require('net');

const DEFAULT_PORT = 3000;
const DEFAULT_BIND_ADDRESS = '127.0.0.1';

function argumentValue(args, name) {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : undefined;
}

function isLoopbackAddress(address) {
  const value = String(address || '').toLowerCase();
  return value === 'localhost' || value === '127.0.0.1' || value === '::1' || value === '[::1]';
}

function isPrivateAddress(address) {
  const value = String(address || '').trim();
  if (value === '0.0.0.0') return true;
  if (net.isIP(value) !== 4) return false;
  const octets = value.split('.').map(Number);
  return octets[0] === 10 ||
    (octets[0] === 192 && octets[1] === 168) ||
    (octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31);
}

function parsePort(value, fallback = DEFAULT_PORT) {
  if (value === undefined || value === null || value === '') return fallback;
  const parsed = Number(value || fallback);
  if (!Number.isInteger(parsed) || parsed <= 0 || parsed > 65535) {
    throw new Error('Desktop port must be an integer between 1 and 65535.');
  }
  return parsed;
}

/**
 * Resolve desktop listener options before starting the bundled server.
 * Non-loopback listeners always require explicit --allow-lan and a private IPv4
 * address (or 0.0.0.0). This pure function is shared by preflight tests and the
 * Electron main process so security checks do not drift between platforms.
 */
function resolveRuntimeConfig({ args = [], env = {}, saved = {} } = {}) {
  const bindAddress = argumentValue(args, '--bind-address') || env.MYAPI_BIND_ADDRESS || saved.bindAddress || DEFAULT_BIND_ADDRESS;
  const port = parsePort(argumentValue(args, '--port') || env.MYAPI_PORT || saved.port);
  const allowLan = args.includes('--allow-lan') || saved.allowLan === true;

  if (!isLoopbackAddress(bindAddress) && !isPrivateAddress(bindAddress)) {
    throw new Error('Desktop LAN binding must use a private IPv4 address or 0.0.0.0.');
  }
  if (!isLoopbackAddress(bindAddress) && !allowLan) {
    throw new Error('LAN sharing is disabled by default; restart with --allow-lan and a private bind address.');
  }

  return {
    bindAddress: String(bindAddress),
    port,
    allowLan,
    isLan: !isLoopbackAddress(bindAddress),
  };
}

module.exports = {
  DEFAULT_BIND_ADDRESS,
  DEFAULT_PORT,
  isLoopbackAddress,
  isPrivateAddress,
  parsePort,
  resolveRuntimeConfig,
};
