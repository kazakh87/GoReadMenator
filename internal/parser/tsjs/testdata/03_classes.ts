// SZENARIO: Klassen mit Basisklasse, Interface-Implementierung, Feldern, Accessoren, statischen Mitgliedern, #privaten Namen, Konstruktor-Parameter-Properties und Decorators
// ABDECKUNG: extends, implements, Property mit Typ, Feld mit Initialisierer, static, #private, get/set-Accessor, Konstruktor mit Parameter-Properties, super-Aufruf, Decorator mit Argumenten, abstract-Klasse, Methodenaufruf im Körper
import { Base } from "./base";

@Injectable()
export abstract class Store extends Base implements Reader, Writer {
  private readonly items: Item[] = [];
  static count = 0;
  #secret: string;
  label?: string;

  constructor(private http: HttpClient, public name = "x") {
    super();
    this.items = load();
  }

  get size(): number {
    return this.items.length;
  }

  set size(value: number) {
    this.count = value;
  }

  async submit(order: Order, retries?: number): Promise<Receipt> {
    const r = await this.http.post("/x", order);
    return new Receipt(r);
  }

  abstract refresh(): void;
}
