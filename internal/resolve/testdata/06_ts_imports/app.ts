import { Repo, helper as h } from "./lib";
import main from "./lib";
import * as lib from "./lib";
import React from "react";

export function run() {
  const r = new Repo();
  h(1);
  main();
  lib.util.keys({});
  React.createElement("div");
  return r;
}
