// SZENARIO: TypeScript mit UTF-8-BOM und Umlauten in Namen
// ABDECKUNG: BOM am Dateianfang, Umlaut in Bezeichner, Interface als Typ
export interface Größe { breite: number }
export function fläche(g: Größe): number { return g.breite; }
