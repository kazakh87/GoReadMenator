// SZENARIO: Neuere TypeScript-Konstrukte: const-Typparameter, accessor-Felder, statische Blöcke, override, satisfies, optional chaining, Nullish-Zuweisung, private Methoden
// ABDECKUNG: class mit const-Typparameter, accessor-Feld, static-Block mit Aufruf, override-Methode, Getter mit optional chaining, satisfies-Ausdruck, Nullish-Ausdruck, #private-Methode, Top-Level using
export class Cache<const T extends object> {
  accessor size = 0;
  static { init(); }
  #secret() { return 1; }
  override run(): void {}
  get value() { return this.size?.toFixed(); }
  total = ((this.size) ?? 0);
  cfg = { port: 80 } satisfies Config;
}
function show(input: Pair<number>) { return input; }
async function main() { await load(); }
using res = openResource();
