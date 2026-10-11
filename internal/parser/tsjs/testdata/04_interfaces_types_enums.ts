// SZENARIO: Interfaces (Erweiterung, Methoden- und Property-Signaturen, optionale Member), Typ-Aliase (Union, Objekt, generisch), Enums (mit Werten), Namespaces und declare module
// ABDECKUNG: interface mit extends, Methodensignatur, Property-Signatur, optionales Member, type-Alias union, type-Alias generisch, enum mit Werten, namespace, declare module, const enum
export interface Repo<T> extends Reader, Writer<T> {
  save(item: T): void;
  readonly id: string;
  find?(id: string): T | undefined;
}

export type Pair<A, B> = [A, B] | { first: A; second: B };
export type Maybe = string | null;

export enum Status { Open = 1, Closed, "Other" = 9 }
export const enum Dir { Up, Down }

export namespace util {
  export function assertNever(x: never): never {
    throw new Error("unexpected");
  }
}

declare module "legacy-lib" {
  export function legacy(): void;
}
