// SZENARIO: Alle Import- und Exportformen, inklusive Re-Exporten, require und dynamischem import()
// ABDECKUNG: default import, named import mit Alias, type-only import, namespace import, side-effect import, import = require, require(), dynamic import(), export * from, export * as, export { } from, Export-Liste mit Alias
import React from "react";
import { useState, type Dispatch, Foo as Bar } from "react/hooks";
import * as utils from "./utils";
import "./polyfill";
import type { Config } from "./config";
import legacy = require("legacy-lib");
const fs = require("fs");
const lazy = import("./lazy");

export * from "./barrel";
export * as ns from "./namespace-reexport";
export { a, b as c } from "./reexports";

export const local = React;
