const dns = require("node:dns");
const https = require("node:https");
const net = require("node:net");
const tls = require("node:tls");
const nodeTimers = require("node:timers");
const { types: { isPromise } } = require("node:util");
const { validatePublicEntry } = require("./public-access-config");

const DEADLINE_MS = 10000;
const MAX_BYTES = 65536;

function ipv4Number(address) {
  return address.split(".").reduce((value, part) => (value << 8n) | BigInt(part), 0n);
}

function ipv6Number(address) {
  let value = address;
  if (value.includes(".")) {
    const split = value.lastIndexOf(":");
    const tail = ipv4Number(value.slice(split + 1));
    value = value.slice(0, split + 1) + (tail >> 16n).toString(16) + ":" + (tail & 65535n).toString(16);
  }
  const halves = value.split("::");
  const left = halves[0] ? halves[0].split(":") : [];
  const right = halves[1] ? halves[1].split(":") : [];
  const words = halves.length === 1 ? left : [...left, ...Array(8 - left.length - right.length).fill("0"), ...right];
  return words.reduce((number, word) => (number << 16n) | BigInt("0x" + word), 0n);
}

function inPrefix(value, base, prefix, bits) {
  const shift = BigInt(bits - prefix);
  return (value >> shift) === (base >> shift);
}

// Conservative ordinary-public destinations, not a routability guarantee.
// IANA registries (2025-10-09):
// https://www.iana.org/assignments/iana-ipv4-special-registry/
// https://www.iana.org/assignments/iana-ipv6-special-registry/
// Special-purpose entries are excluded even when globally reachable.
const V4_SPECIAL = [
  ["0.0.0.0", 8], ["10.0.0.0", 8], ["100.64.0.0", 10], ["127.0.0.0", 8],
  ["169.254.0.0", 16], ["172.16.0.0", 12], ["192.0.0.0", 24], ["192.0.2.0", 24],
  ["192.31.196.0", 24], ["192.52.193.0", 24], ["192.88.99.0", 24], ["192.168.0.0", 16],
  ["192.175.48.0", 24], ["198.18.0.0", 15], ["198.51.100.0", 24], ["203.0.113.0", 24],
  ["224.0.0.0", 4], ["240.0.0.0", 4],
].map(([base, prefix]) => [ipv4Number(base), prefix]);
const V6_SPECIAL = [
  ["2001::", 23], ["2001:db8::", 32], ["2002::", 16], ["2620:4f:8000::", 48], ["3fff::", 20],
].map(([base, prefix]) => [ipv6Number(base), prefix]);

function publicAddress(address, family) {
  if (typeof address !== "string" || address.includes("%") || net.isIP(address) !== family) return false;
  if (family === 4) {
    const value = ipv4Number(address);
    return !V4_SPECIAL.some(([base, prefix]) => inPrefix(value, base, prefix, 32));
  }
  if (family !== 6) return false;
  const value = ipv6Number(address);
  return inPrefix(value, ipv6Number("2000::"), 3, 128) &&
    !V6_SPECIAL.some(([base, prefix]) => inPrefix(value, base, prefix, 128));
}

function tlsFailure(error) {
  const code = error?.code;
  if (code === "ERR_TLS_CERT_ALTNAME_INVALID") return "tls-hostname";
  if (code === "CERT_HAS_EXPIRED" || code === "CERT_NOT_YET_VALID") return "tls-validity";
  if (["DEPTH_ZERO_SELF_SIGNED_CERT", "SELF_SIGNED_CERT_IN_CHAIN", "UNABLE_TO_VERIFY_LEAF_SIGNATURE",
    "UNABLE_TO_GET_ISSUER_CERT", "UNABLE_TO_GET_ISSUER_CERT_LOCALLY", "CERT_UNTRUSTED", "CERT_REJECTED",
    "CERT_SIGNATURE_FAILURE"].includes(code)) return "tls-untrusted";
  return null;
}

/** Internal DI is for module tests only. IPC must never select these services. */
function createPublicAccessCheck(dependencies = {}) {
  if (!dependencies || typeof dependencies !== "object" || Array.isArray(dependencies) ||
    Object.keys(dependencies).some(key => !["lookup", "request", "timers", "now"].includes(key))) {
    throw new TypeError("Invalid public entry check dependencies.");
  }
  const lookup = dependencies.lookup === undefined ? (hostname, options) => dns.promises.lookup(hostname, options) : dependencies.lookup;
  const request = dependencies.request === undefined ? https.request : dependencies.request;
  const timers = dependencies.timers === undefined ? nodeTimers : dependencies.timers;
  const now = dependencies.now === undefined ? Date.now : dependencies.now;
  if (typeof lookup !== "function" || typeof request !== "function" || typeof now !== "function" ||
    !timers || typeof timers.setTimeout !== "function" || typeof timers.clearTimeout !== "function") {
    throw new TypeError("Invalid public entry check dependencies.");
  }

  function check(origin, options = {}) {
    const validOptions = options !== null && typeof options === "object" && !Array.isArray(options) &&
      Object.keys(options).every(key => key === "signal" || key === "onProgress");
    const signal = validOptions ? options.signal : undefined;
    const onProgress = validOptions ? options.onProgress : undefined;
    const hasSignal = signal instanceof AbortSignal;
    return new Promise((resolve) => {
      const report = {
        schemaVersion: 1, origin: null, outcome: "unknown", code: "checking", checkedAt: null,
        source: "this-computer", externalVerification: "unknown", sameInstanceVerified: false,
        modelVerified: false, quotaVerified: false,
        dns: { state: "not-run", source: null }, tls: { state: "not-run" },
        status: { state: "not-run", version: null, productEdition: null },
      };
      let settled = false, timer = null, outgoing = null, incoming = null;
      const finish = (outcome, code) => {
        if (settled) return;
        settled = true;
        if (timer !== null) timers.clearTimeout(timer);
        if (hasSignal) signal.removeEventListener("abort", abort);
        if (incoming) incoming.destroy();
        if (outgoing) outgoing.destroy();
        if (signal?.aborted) { outcome = "cancelled"; code = "cancelled"; }
        for (const stage of [report.dns, report.tls, report.status]) {
          if (stage.state === "checking") stage.state = "unknown";
        }
        report.outcome = outcome; report.code = code;
        let timestamp;
        try { timestamp = now(); } catch { timestamp = null; }
        report.checkedAt = Number.isSafeInteger(timestamp) && timestamp >= 0 ? timestamp : null;
        if (report.checkedAt === null && outcome === "passed") { report.outcome = "unknown"; report.code = "clock-unavailable"; }
        Object.freeze(report.dns); Object.freeze(report.tls); Object.freeze(report.status);
        resolve(Object.freeze(report));
      };
      const abort = () => finish("cancelled", "cancelled");
      const progress = () => {
        if (settled) return false;
        if (signal?.aborted) { abort(); return false; }
        if (!onProgress) return true;
        try {
          const returned = onProgress(structuredClone(report));
          if (isPromise(returned) || returned && typeof returned.then === "function") {
            // Progress is synchronous. Consume a rejected promise but never
            // let an async callback race a later successful publication.
            if (isPromise(returned)) Promise.prototype.then.call(returned, undefined, () => {});
            finish("unknown", "progress-unavailable");
          }
        } catch { finish("unknown", "progress-unavailable"); }
        return !settled;
      };
      if (!validOptions || signal !== undefined && !hasSignal || onProgress !== undefined && typeof onProgress !== "function") {
        finish("unknown", "invalid-options"); return;
      }
      if (signal?.aborted) { abort(); return; }
      const parsed = validatePublicEntry(origin);
      if (!parsed.ok) { finish("failed", "invalid-entry"); return; }
      report.origin = parsed.origin;
      const url = new URL(parsed.origin);
      const hostname = url.hostname.replace(/^\[|\]$/g, "");
      const literalFamily = net.isIP(hostname);
      signal?.addEventListener("abort", abort, { once: true });
      timer = timers.setTimeout(() => finish("unknown", "timeout"), DEADLINE_MS);
      report.dns.state = "checking"; report.dns.source = literalFamily ? "literal" : "lookup";
      if (!progress()) return;
      const readAddresses = literalFamily
        ? Promise.resolve([{ address: hostname, family: literalFamily }])
        : Promise.resolve().then(() => settled ? null : lookup(hostname, { all: true, verbatim: true }));
      readAddresses.then((addresses) => {
        if (settled) return;
        if (!Array.isArray(addresses) || addresses.length === 0 || addresses.length > 64 ||
          addresses.some(item => !item || ![4, 6].includes(item.family) || typeof item.address !== "string" || net.isIP(item.address) !== item.family)) {
          report.dns.state = "unknown"; progress(); finish("unknown", "dns-unavailable"); return;
        }
        if (addresses.some(item => !publicAddress(item.address, item.family))) {
          report.dns.state = "failed"; progress(); finish("failed", "address-type-unsupported"); return;
        }
        const selected = { address: addresses[0].address, family: addresses[0].family };
        report.dns.state = "passed";
        if (!progress()) return;
        report.tls.state = "checking";
        if (!progress()) return;
        const fail = error => {
          if (settled) return;
          const code = tlsFailure(error);
          if (report.tls.state !== "passed") {
            report.tls.state = code ? "failed" : "unknown";
            progress(); finish(code ? "failed" : "unknown", code || "network-unavailable");
          } else {
            report.status.state = "unknown"; progress(); finish("unknown", "response-unavailable");
          }
        };
        const options = {
          protocol: "https:", hostname, port: url.port || 443, method: "GET", path: "/api/status",
          headers: { Accept: "application/json" }, agent: false, rejectUnauthorized: true,
          servername: literalFamily ? "" : hostname, family: selected.family, autoSelectFamily: false,
          maxHeaderSize: 16384,
          checkServerIdentity: (_serverName, certificate) => tls.checkServerIdentity(hostname, certificate),
          lookup: (requested, lookupOptions, callback) => {
            if (typeof lookupOptions === "function") { callback = lookupOptions; lookupOptions = {}; }
            if (settled || signal?.aborted || requested !== hostname || lookupOptions?.family && lookupOptions.family !== selected.family) {
              callback(new Error("Pinned public destination unavailable.")); return;
            }
            if (lookupOptions?.all) callback(null, [{ ...selected }]);
            else callback(null, selected.address, selected.family);
          },
        };
        try {
          outgoing = request(options, (response) => {
            incoming = response;
            response.on("error", fail);
            if (settled) { response.destroy(); return; }
            if (report.tls.state !== "passed" || report.status.state !== "checking") { finish("unknown", "tls-unverified"); return; }
            const status = response.statusCode;
            if (!Number.isInteger(status) || status < 100 || status > 599) { report.status.state = "unknown"; progress(); finish("unknown", "response-unavailable"); return; }
            if (status !== 200) {
              report.status.state = "failed"; progress();
              finish("failed", status >= 300 && status < 400 ? "http-redirect" : status === 401 || status === 403 ? "http-auth-required" : "http-status"); return;
            }
            const chunks = [];
            let size = 0;
            response.on("data", (chunk) => {
              if (settled) return;
              const bytes = Buffer.from(chunk);
              size += bytes.length;
              if (size > MAX_BYTES) { report.status.state = "failed"; progress(); finish("failed", "response-too-large"); return; }
              chunks.push(bytes);
            });
            response.on("aborted", () => fail());
            response.on("close", () => { if (!settled && !response.complete) fail(); });
            response.on("end", () => {
              if (settled) return;
              if (!response.complete) { fail(); return; }
              let body;
              try { body = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(Buffer.concat(chunks))); } catch { body = null; }
              const version = body?.data?.version;
              const edition = body?.data?.product_edition;
              if (body?.success !== true || !body.data || Array.isArray(body.data) || typeof version !== "string" || version.length > 96 ||
                !/^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(version) || !["full", "lite"].includes(edition)) {
                report.status.state = "failed"; progress(); finish("failed", "invalid-status"); return;
              }
              report.status.state = "passed"; report.status.version = version; report.status.productEdition = edition;
              if (progress()) finish("passed", "ok");
            });
          });
          outgoing.on("error", fail);
          if (settled || signal?.aborted) { if (!settled) abort(); outgoing.destroy(); return; }
          outgoing.on("close", () => { if (!settled && !incoming) fail(); });
          outgoing.once("socket", (socket) => {
            socket.once("secureConnect", () => {
              if (settled) return;
              let certificate, nameError, timestamp;
              try { certificate = socket.getPeerCertificate(); nameError = tls.checkServerIdentity(hostname, certificate); timestamp = now(); }
              catch { finish("unknown", "tls-unverified"); return; }
              if (!socket.encrypted || socket.authorized !== true) {
                const code = tlsFailure({ code: socket.authorizationError });
                report.tls.state = code ? "failed" : "unknown"; progress(); finish(code ? "failed" : "unknown", code || "tls-unverified"); return;
              }
              if (nameError) { report.tls.state = "failed"; progress(); finish("failed", "tls-hostname"); return; }
              const from = Date.parse(certificate.valid_from), to = Date.parse(certificate.valid_to);
              if (!Number.isFinite(from) || !Number.isFinite(to) || !Number.isSafeInteger(timestamp)) { finish("unknown", "tls-unverified"); return; }
              if (timestamp < from || timestamp > to) { report.tls.state = "failed"; progress(); finish("failed", "tls-validity"); return; }
              report.tls.state = "passed";
              if (!progress()) return;
              report.status.state = "checking";
              if (progress()) outgoing.end();
            });
          });
        } catch { fail(); }
      }, () => {
        if (settled) return;
        report.dns.state = "unknown"; progress(); finish("unknown", "dns-unavailable");
      }).catch(() => { if (!settled) finish("unknown", "network-unavailable"); });
    });
  }
  return Object.freeze({ check });
}

module.exports = { createPublicAccessCheck };
