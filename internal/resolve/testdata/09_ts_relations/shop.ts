import { Money, Priced, Base } from "./types";

export class Product extends Base implements Priced {
  cost: Money;
  price(): Money { return this.cost; }
  discount(by: Money): Money { return by; }
}
