// SZENARIO: Funktionsdeklarationen, Überladungen, Pfeilfunktionen, function-Ausdrücke, async, Generics und Default-Export
// ABDECKUNG: function declaration, Überladungssignaturen, implementierung, const-Pfeilfunktion, Pfeilfunktion ohne Klammer, async-Pfeil, function-Ausdruck, generische Funktion, default export, Variable ohne Funktionswert
export function add(a: number, b: number): number {
  return a + b;
}

export function pick(x: string): string;
export function pick(x: number): number;
export function pick(x: any): any {
  return x;
}

export const double = (n: number): number => n * 2;
const greet = async name => {
  return helper(name);
};
const handler = function (req: Request) {
  return req.url;
};
export function identity<T>(value: T): T {
  return value;
}
export default function main() {
  return 1;
}
export const answer = 42;
function helper(s) { return s; }
