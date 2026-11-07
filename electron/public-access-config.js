(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.MyAPIPublicAccess = api;
})(typeof window !== "undefined" ? window : globalThis, function () {
  "use strict";

  const MAX_PUBLIC_ORIGIN_LENGTH = 2048;
  const MAX_PUBLIC_BASE_URL_LENGTH = MAX_PUBLIC_ORIGIN_LENGTH + 3;

  function localHostname(hostname) {
    const host = hostname
      .toLowerCase()
      .replace(/^\[|\]$/g, "")
      .replace(/\.+$/, "");
    if (
      host === "localhost" ||
      host.endsWith(".localhost") ||
      host === "local" ||
      host.endsWith(".local")
    )
      return true;
    if (
      ["::", "::1"].includes(host) ||
      /^(fc|fd)[0-9a-f]{2}:/.test(host) ||
      /^fe[89ab][0-9a-f]:/.test(host)
    )
      return true;
    const parts = host.split(".");
    if (parts.length === 4 && parts.every((part) => /^\d+$/.test(part) && Number(part) <= 255)) {
      const [first, second] = parts.map(Number);
      return (
        first === 0 ||
        first === 10 ||
        first === 127 ||
        (first === 172 && second >= 16 && second <= 31) ||
        (first === 192 && second === 168) ||
        (first === 169 && second === 254)
      );
    }
    // WHATWG URL normalizes IPv4-mapped IPv6 to hexadecimal groups.
    if (host.startsWith("::ffff:")) {
      const mapped = host.slice(7).split(":");
      if (mapped.length === 2 && mapped.every((part) => /^[0-9a-f]{1,4}$/.test(part))) {
        const first = parseInt(mapped[0], 16);
        const second = parseInt(mapped[1], 16);
        return localHostname(`${first >> 8}.${first & 255}.${second >> 8}.${second & 255}`);
      }
    }
    return false;
  }

  function parseRoot(value) {
    if (typeof value !== "string") return { ok: false, code: "invalid" };
    if (value.length > MAX_PUBLIC_ORIGIN_LENGTH) return { ok: false, code: "too-long" };
    if (/[\u0000-\u001f\u007f]/.test(value)) return { ok: false, code: "control-character" };
    if (value.includes("\\")) return { ok: false, code: "backslash" };
    if (/[?#]/.test(value)) return { ok: false, code: "query-fragment" };
    const input = value.trim();
    if (!input) return { ok: false, code: "empty" };
    const authority = /^[a-z][a-z\d+.-]*:\/\/([^/]*)(.*)$/i.exec(input);
    if (authority && authority[1].includes("@")) return { ok: false, code: "credentials" };
    let url;
    try {
      url = new URL(input);
    } catch {
      return { ok: false, code: "invalid" };
    }
    if (url.username || url.password) return { ok: false, code: "credentials" };
    if (!["http:", "https:"].includes(url.protocol) || !authority)
      return { ok: false, code: "scheme" };
    // Check the original suffix too: /., /v1 and encoded paths must not be
    // silently normalized or removed when constructing a client Base URL.
    if ((authority[2] !== "" && authority[2] !== "/") || url.pathname !== "/")
      return { ok: false, code: "path" };
    if (!url.hostname) return { ok: false, code: "invalid" };
    return { ok: true, url };
  }

  function validatePublicEntry(value) {
    const parsed = parseRoot(value);
    if (!parsed.ok) return parsed;
    if (localHostname(parsed.url.hostname)) return { ok: false, code: "local-or-lan" };
    if (parsed.url.protocol !== "https:") return { ok: false, code: "scheme" };
    const origin = parsed.url.origin;
    if (origin.length > MAX_PUBLIC_ORIGIN_LENGTH) return { ok: false, code: "too-long" };
    return { ok: true, code: "valid", origin, baseURL: `${origin}/v1` };
  }

  function validateProxyTarget(value) {
    const parsed = parseRoot(value);
    return parsed.ok && localHostname(parsed.url.hostname) && parsed.url.origin === value;
  }

  return Object.freeze({
    MAX_PUBLIC_ORIGIN_LENGTH,
    MAX_PUBLIC_BASE_URL_LENGTH,
    validatePublicEntry,
    validateProxyTarget,
  });
});
