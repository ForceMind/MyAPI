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

/**
 * Build a user-facing LAN status snapshot for the desktop tray dialog.
 *
 * The desktop listener is resolved once during process startup.  This helper
 * deliberately exposes that immutable state and a restart hint instead of
 * pretending that the listener can be changed while the backend is running.
 */
function describeRuntimeConfig(config, platform = process.platform) {
  const bindAddress = String(config?.bindAddress || DEFAULT_BIND_ADDRESS);
  const port = Number(config?.port || DEFAULT_PORT);
  const lanEnabled = Boolean(config?.isLan && config?.allowLan);
  const displayHost = bindAddress === '0.0.0.0' ? '<private-LAN-IP>' : bindAddress;
  const endpoint = `http://${displayHost}:${port}`;

  const firewallHint = platform === 'win32'
    ? 'Windows：如同事无法连接，请在 Windows Defender 防火墙中允许 MyAPI 访问“专用网络”。'
    : platform === 'darwin'
      ? 'macOS：如同事无法连接，请在“系统设置 > 网络 > 防火墙”中允许 MyAPI 接受传入连接。'
      : 'Linux：如同事无法连接，请检查 ufw/firewalld 是否允许该端口，并仅开放可信的局域网网段。';

  return {
    bindAddress,
    port,
    endpoint,
    lanEnabled,
    mode: lanEnabled ? 'LAN（私有网络）' : '仅本机（回环地址）',
    firewallHint,
    restartHint: '监听地址和端口在启动时确定。修改参数后请退出 MyAPI，再使用新的参数重新启动；不会在运行中动态切换。',
  };
}

/** Return an IPv4 address that can be used to probe the effective listener. */
function getHealthCheckAddress(bindAddress) {
  return String(bindAddress || DEFAULT_BIND_ADDRESS) === '0.0.0.0'
    ? '127.0.0.1'
    : String(bindAddress || DEFAULT_BIND_ADDRESS);
}

module.exports = {
  DEFAULT_BIND_ADDRESS,
  DEFAULT_PORT,
  isLoopbackAddress,
  isPrivateAddress,
  parsePort,
  resolveRuntimeConfig,
  describeRuntimeConfig,
  getHealthCheckAddress,
};
