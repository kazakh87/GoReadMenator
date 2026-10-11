// SZENARIO: JSX-Komponenten in .tsx: Komponenten werden als Verwendungen erkannt, Kleinbuchstaben-Tags (HTML) nicht. Apostrophe im Text stören nicht.
// ABDECKUNG: JSX-Komponente als Verwendung, eigenes Element ohne Komponente, Attribute, Text mit Apostroph, verschachtelte Komponenten
import Button from "./Button";
import { Card } from "./Card";

export function App() {
  return (
    <div>
      <Button label="ok" />
      <span>Don't stop</span>
      <Card>
        <Button label="inner" />
      </Card>
    </div>
  );
}
