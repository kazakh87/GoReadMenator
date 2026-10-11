#!/usr/bin/env node
// SZENARIO: JavaScript ohne Typen: Shebang, CommonJS-Module, Klassen mit Methoden, Pfeilfunktionen und Export-Zuweisung
// ABDECKUNG: Shebang-Zeile, require mit Zuweisung, Klasse ohne Typen, Methode, Pfeilfunktion, module.exports
const path = require("path");

class Cache {
  constructor(size) {
    this.size = size;
  }
  get(key) {
    return this.map[key];
  }
}

const make = (n) => new Cache(n);
module.exports = { Cache, make, path };
