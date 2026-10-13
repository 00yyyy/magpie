// Run with Node's test runner; needs no browser. A key written twice in one
// language is a trap: the later one silently wins, so an edit to the first
// does nothing. Every key appears once per language, in i18n.js or in one
// plugin's i18n.js (strings.cjs), never in two of them, and a plugin's
// file says every one of its keys in every language.
const assert = require("node:assert/strict");
const { test } = require("node:test");
const { strings } = require("./strings.cjs");

test("no language has a key twice", () => {
  const { files, twice } = strings();
  for (const { file, src } of files) {
    const body = file === "i18n.js" ? src.slice(0, src.indexOf("\n};")) : src;
    let lang, seen;
    body.split("\n").forEach((line, i) => {
      const head = line.match(/^  "?([\w-]+)"?: \{/);
      if (head) return void ((lang = head[1]), (seen = new Map()));
      const key = line.match(/^    ("(?:[^"\\]|\\.)*")\s*:/);
      if (!key || !lang) return;
      if (seen.has(key[1])) twice.push(`${file} ${lang} ${key[1]}: lines ${seen.get(key[1])} and ${i + 1}`);
      else seen.set(key[1], i + 1);
    });
  }
  assert.deepEqual(twice, []);
});

test("a plugin's words are said in every language", () => {
  for (const { file, dict } of strings().files.slice(1)) {
    assert.deepEqual(Object.keys(dict).sort(), ["de", "ja", "zh", "zh-TW"], file);
    const zh = Object.keys(dict.zh);
    assert.ok(zh.length, `${file} says nothing`);
    for (const lang of ["zh-TW", "ja", "de"]) assert.deepEqual(Object.keys(dict[lang]).sort(), [...zh].sort(), `${file} ${lang}`);
  }
});
