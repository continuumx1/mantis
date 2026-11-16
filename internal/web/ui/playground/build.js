// build.js — generates playground.html from index.html, so the Playground is
// always the current app: the same markup, styles and scripts, plus a banner
// and playground-api.js (which answers /api/* from recorded sample data).
//
//   node internal/web/ui/playground/build.js          write ../playground.html
//   node internal/web/ui/playground/build.js --check  exit 1 if it is stale
"use strict";
const fs = require("fs");
const path = require("path");

const UI = path.join(__dirname, "..");
const SITE = "https://mantis.continuumx1.com";

function replaceOnce(s, find, repl) {
  const i = s.indexOf(find);
  if (i < 0 || s.indexOf(find, i + find.length) >= 0) throw new Error("index.html changed: expected exactly one " + JSON.stringify(find));
  return s.slice(0, i) + repl + s.slice(i + find.length);
}

function build(index) {
  let s = index;
  s = replaceOnce(s, "<!doctype html>\n", "<!doctype html>\n<!-- Generated from index.html by playground/build.js. Do not edit; edit index.html and rebuild. -->\n");
  s = replaceOnce(s, "<title>Mantis</title>", `<title>Mantis Playground</title>
<meta name="description" content="Try the Mantis Kubernetes topology and observability app on a sample cluster, in your browser.">`);
  s = replaceOnce(s, "</head>", `<style>
  /* Playground banner: a fixed bar above the app, which moves down below it. */
  .pg-bar { position: fixed; top: 0; left: 0; right: 0; height: 40px; z-index: 60; display: flex; align-items: center; gap: 14px;
    padding: 0 14px; background: #10150d; color: #d9e5c9; font: 13px/1.3 var(--m-font, system-ui, sans-serif); border-bottom: 1px solid #2c3a1c; }
  .pg-bar b { color: #a6e22e; font-weight: 650; }
  .pg-bar .pg-msg { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .pg-bar a { color: #a6e22e; text-decoration: none; font-weight: 600; white-space: nowrap; }
  .pg-bar a:hover { text-decoration: underline; }
  .pg-bar .pg-back { color: #d9e5c9; border: 1px solid #3c4d28; border-radius: 6px; padding: 3px 9px; }
  .pg-bar .pg-cta { background: #a6e22e; color: #10150d; border-radius: 6px; padding: 4px 10px; }
  .m-shell { top: 40px; }
  /* There is no account to sign out of. */
  #mSignOut { display: none; }
  @media (max-width: 760px) { .pg-bar .pg-long { display: none; } }
</style>
</head>`);
  s = replaceOnce(s, "<body>\n", `<body>
<div class="pg-bar" role="note">
  <a class="pg-back" href="${SITE}">← Mantis</a>
  <span class="pg-msg"><b>Mantis Playground</b> · the real Mantis app on a sample cluster<span class="pg-long"> — recorded data, nothing here is live</span></span>
  <a class="pg-cta" href="${SITE}/#get-started" target="_blank" rel="noopener noreferrer">Get Mantis →</a>
</div>
`);
  s = replaceOnce(s, '<script src="graph-engine.js"></script>', '<script src="playground-api.js"></script>\n<script src="graph-engine.js"></script>');
  return s;
}

function main() {
  const index = fs.readFileSync(path.join(UI, "index.html"), "utf8");
  const html = build(index);
  const target = path.join(UI, "playground.html");
  if (process.argv.includes("--check")) {
    const current = fs.existsSync(target) ? fs.readFileSync(target, "utf8") : "";
    if (current !== html) {
      console.error("playground.html is stale: run node internal/web/ui/playground/build.js");
      process.exit(1);
    }
  } else {
    fs.writeFileSync(target, html);
    console.log("wrote", path.relative(process.cwd(), target));
  }
}

if (require.main === module) main();
module.exports = { build, UI };
