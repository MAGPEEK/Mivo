// Evaluates the share-name validator of an install wizard file the way a browser does: with the
// JavaScript RegExp engine (scripts/package-files.bats, task SPK-06).
//
//   node share-name-check.js <install_uifile>  <each name followed by a NUL byte on stdin>
//
// Prints one line per name, "valid" or "invalid", in input order. The validator's "expr" is a
// regex literal such as "/^a$/i"; DSM's wizard turns it into a RegExp.
"use strict";

const fs = require("node:fs");

function shareNameSubitem(wizard) {
  for (const step of wizard) {
    for (const item of step.items ?? []) {
      for (const subitem of item.subitems ?? []) {
        if (subitem.key === "wizard_media_share") {
          return subitem;
        }
      }
    }
  }
  throw new Error("no subitem with key wizard_media_share");
}

function regexFromLiteral(literal) {
  const match = /^\/(.*)\/([a-z]*)$/s.exec(literal);
  if (match === null) {
    throw new Error(`validator expr is not a regex literal: ${literal}`);
  }
  return new RegExp(match[1], match[2]);
}

const wizard = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const pattern = regexFromLiteral(shareNameSubitem(wizard).validator.regex.expr);
// Every name ends with a NUL byte, so the last element of the split is always empty.
const names = fs.readFileSync(0, "utf8").split("\0").slice(0, -1);
for (const name of names) {
  console.log(pattern.test(name) ? "valid" : "invalid");
}
