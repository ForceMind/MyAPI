const net = require('net');
const os = require('os');

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

/**
 * Return usable RFC1918 IPv4 addresses exposed by the host network stack.
 *
 * The optional argument keeps this helper deterministic in tests and avoids
 * making the desktop UI depend on any provider or credential files.  Loopback,
 * internal, wildcard, IPv6 and public addresses are intentionally excluded.
 */
function getPrivateIPv4Candidates(networkInterfaces = os.networkInterfaces()) {
  const candidates = [];
  for (const entries of Object.values(networkInterfaces || {})) {
    if (!Array.isArray(entries)) continue;
    for (const entry of entries) {
      if (!entry || entry.internal) continue;
      const address = String(entry.address || '').trim();
      if (!address || address === '0.0.0.0' || net.isIP(address) !== 4 || !isPrivateAddress(address)) continue;
      if (!candidates.includes(address)) candidates.push(address);
    }
  }
  return candidates;
}

function replaceArgument(args, name, value) {
  const result = [];
  for (let index = 0; index < args.length; index += 1) {
    if (args[index] === name) {
      index += 1;
      continue;
    }
    result.push(args[index]);
  }
  if (value !== undefined) result.push(name, String(value));
  return result;
}

/**
 * Build a safe Electron relaunch argument list.  The returned list is suitable
 * for app.relaunch({ args }) and always carries an explicit bind address, so
 * environment variables cannot silently widen a loopback-only restart.
 */
function buildRelaunchArgs(args = [], { bindAddress = DEFAULT_BIND_ADDRESS, port, allowLan = false } = {}) {
  if (!isLoopbackAddress(bindAddress) && (!allowLan || !isPrivateAddress(bindAddress))) {
    throw new Error('Relaunch LAN binding must use a private IPv4 address and explicit LAN opt-in.');
  }
  let result = Array.from(args);
  result = replaceArgument(result, '--bind-address', bindAddress);
  result = replaceArgument(result, '--port', port);
  result = result.filter((argument) => argument !== '--allow-lan');
  if (allowLan) result.push('--allow-lan');
  return result;
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
function describeRuntimeConfig(config, platform = process.platform, networkInterfaces) {
  const bindAddress = String(config?.bindAddress || DEFAULT_BIND_ADDRESS);
  const port = Number(config?.port || DEFAULT_PORT);
  const lanEnabled = Boolean(config?.isLan && config?.allowLan);
  const endpointCandidates = bindAddress === '0.0.0.0'
    ? getPrivateIPv4Candidates(networkInterfaces)
    : [bindAddress];
  const endpoints = endpointCandidates.map((host) => `http://${host}:${port}`);
  const endpoint = endpoints.length > 0
    ? endpoints.join(', ')
    : `No RFC1918 address detected (port ${port})`;

  const firewallHint = platform === 'win32'
    ? 'Windows：如同事无法连接，请在 Windows Defender 防火墙中允许 MyAPI 访问“专用网络”。'
    : platform === 'darwin'
      ? 'macOS：如同事无法连接，请在“系统设置 > 网络 > 防火墙”中允许 MyAPI 接受传入连接。'
      : 'Linux：如同事无法连接，请检查 ufw/firewalld 是否允许该端口，并仅开放可信的局域网网段。';

  return {
    bindAddress,
    port,
    endpoint,
    endpoints,
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

/**
 * Only successful HTTP responses prove that a listener is the expected
 * service.  A 404/redirect from an unrelated process on the configured port
 * must keep the desktop startup probe in its retry path.
 */
function isSuccessfulHttpStatus(statusCode) {
  const value = Number(statusCode);
  return Number.isInteger(value) && value >= 200 && value < 300;
}

/**
 * Validate the backend status payload used by the production startup probe.
 * Development mode probes an HTML frontend path and therefore continues to
 * use the status-only helper above.
 */
function isSuccessfulStatusResponse(statusCode, body) {
  if (!isSuccessfulHttpStatus(statusCode)) return false;
  try {
    const payload = JSON.parse(String(body || ''));
    return payload && payload.success === true;
  } catch {
    return false;
  }
}

module.exports = {
  DEFAULT_BIND_ADDRESS,
  DEFAULT_PORT,
  isLoopbackAddress,
  isPrivateAddress,
  getPrivateIPv4Candidates,
  buildRelaunchArgs,
  parsePort,
  resolveRuntimeConfig,
  describeRuntimeConfig,
  getHealthCheckAddress,
  isSuccessfulHttpStatus,
  isSuccessfulStatusResponse,
};
