import { Options, make } from "@core/index";
import { build } from "@core/model";

export const opts: Options = { verbose: true };
export const a = make(opts);
export const b = build(opts);
