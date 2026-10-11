export function refresh() { return 2; }
export function user(c: any) {
  c.get("x");     // get existiert nur einmal im Projekt: HEURISTIC
  refresh();      // mehrdeutig: zwei Funktionen mit diesem Namen
}
