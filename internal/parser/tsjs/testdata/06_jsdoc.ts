// SZENARIO: JSDoc mit Beschreibung, @param (auch optional und mit Standardwert), @returns, @remarks und @see/{@link}
// ABDECKUNG: Summary, @param mit Typ, @param optional in eckigen Klammern, @returns, @remarks, @see, {@link} im Fließtext
/**
 * Summiert Werte.
 * @param a Erster Wert
 * @param {number} [b=2] Zweiter Wert
 * @returns Die Summe.
 * @remarks Nur ganze Zahlen, siehe {@link other.run}.
 * @see other.run
 */
export function sum(a: number, b?: number) {
  return a + (b ?? 0);
}
