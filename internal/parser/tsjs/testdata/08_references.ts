// SZENARIO: Verwendungen im Körper: Aufrufe mit Qualifier (Namespace-Import), new, Schreibzugriffe, Typverweise bei as-Casts und Typargumenten, Argumentzahlen
// ABDECKUNG: Aufruf, Qualifier-Aufruf über Namespace-Import, new-Ausdruck, this-Zuweisung, lokale Deklaration (keine Schreibung), as-Cast Typverweis, generischer Aufruf, Argumentzahl
import * as util from "./util";
import { Helper } from "./helper";

export class Client {
  private helper = new Helper();
  count = 0;

  run(input: unknown) {
    const x = util.parse(input);
    this.count = 2;
    const h = new Helper(1, 2);
    const y = input as Payload;
    return pick<number>(x, y, h);
  }
}
