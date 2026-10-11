// SZENARIO: Texte und Literale dürfen keine Symbole erzeugen. Division und Regex werden unterschieden, Template-Interpolationen bleiben sichtbar.
// ABDECKUNG: Zeilenkommentar, Blockkommentar, Strings mit Escapes, Template-Literal, Interpolation mit Aufruf, verschachtelte Template-Literale, Regex-Literal mit Klassen, Division zweier Zahlen
// function FakeInLineComment() {}
/* function FakeInBlockComment() {} */
const s1 = "function FakeInString() {}";
const s2 = 'class FakeSingle {}';
const t1 = `template class FakeTpl {} ${realCall(1)} tail ${`inner ${deepCall()}`}`;
const re = /function FakeRegex\/x[a-z]{}/g;
const div = 10 / 2 / 5;
export function real() {}
