const fs = require("fs");
const path = require("path");

function normalizeLanguage(value) {
  return String(value || "")
    .toLowerCase()
    .startsWith("zh")
    ? "zh"
    : "en";
}

function createNativeI18n({ directory = path.join(__dirname, "locales"), language = "en" } = {}) {
  const dictionaries = {};
  for (const locale of ["en", "zh"]) {
    try {
      const resource = JSON.parse(fs.readFileSync(path.join(directory, `${locale}.json`), "utf8"));
      const source = resource && resource.translation;
      dictionaries[locale] = Object.fromEntries(
        Object.entries(source || {}).filter(([, value]) => typeof value === "string"),
      );
    } catch {
      dictionaries[locale] = {};
    }
  }
  let selected = normalizeLanguage(language);
  return {
    get language() {
      return selected;
    },
    setLanguage(value) {
      selected = normalizeLanguage(value);
    },
    t(key, values = {}) {
      const template = dictionaries[selected][key] || dictionaries.en[key] || key;
      return template.replace(/\{\{([a-zA-Z0-9_]+)\}\}/g, (token, name) =>
        Object.prototype.hasOwnProperty.call(values, name) ? String(values[name]) : token,
      );
    },
  };
}

module.exports = { createNativeI18n, normalizeLanguage };
