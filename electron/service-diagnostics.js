const MAX_DIAGNOSTIC_CHARACTERS = 65536;

// Classifications describe startup evidence, not an automatic repair plan.
function classifyServiceError(text) {
  const value = String(text || "").toLowerCase();
  if (/address already in use|eaddrinuse|failed to start http server/.test(value)) return "port";
  if (/database is locked|unable to open database|sqlite_cantopen|sqlite_busy/.test(value))
    return "database";
  if (/permission denied|access denied|eacces|eperm/.test(value)) return "permission";
  if (/network is unreachable|no such host|connection refused|enotfound|econnrefused/.test(value))
    return "network";
  if (/invalid configuration|failed to parse config|configuration.*(yaml|json).*parse/.test(value))
    return "configuration";
  if (/out of memory|cannot allocate memory|enomem/.test(value)) return "memory";
  if (/no such file or directory|cannot find the file|enoent/.test(value)) return "missing-file";
  return "unknown";
}

function createServiceDiagnostics(generation) {
  let text = "";
  let truncated = false;
  return {
    generation,
    append(value) {
      const next = text + String(value || "");
      if (next.length > MAX_DIAGNOSTIC_CHARACTERS) truncated = true;
      text = next.slice(-MAX_DIAGNOSTIC_CHARACTERS);
    },
    category() {
      return classifyServiceError(text);
    },
    hasOriginal() {
      return text.length > 0;
    },
    originalText() {
      return text;
    },
    isTruncated() {
      return truncated;
    },
  };
}

module.exports = { MAX_DIAGNOSTIC_CHARACTERS, classifyServiceError, createServiceDiagnostics };
