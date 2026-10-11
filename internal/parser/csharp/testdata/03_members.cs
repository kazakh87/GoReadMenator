// SZENARIO: Alle Member-Arten: Felder (mehrere Deklaratoren), Properties (auto, Ausdruck, init), Indexer, Events, Methoden (Ausdruckskörper, generisch, mehrzeilig), Konstruktoren, Operatoren, Destruktor
// ABDECKUNG: field, multiple declarators, auto-property, expression-bodied property, init-accessor, indexer, event, method, generic method, expression-bodied method, constructor, static constructor, operator, conversion operator, destructor
namespace Shop;

public class Account
{
    private int _a = 1, _b;
    public const int MaxSize = 10;
    public string Name { get; set; } = "x";
    public int Total => _a + _b;
    public string Code { get; init; }
    public int this[int index] { get { return index; } }
    public event EventHandler Changed;

    static Account() { }

    public Account(int a)
    {
        _a = a;
    }

    ~Account() { }

    public void Reset() { _a = 0; }
    public T Convert<T>(object value) => (T)value;
    public int Sum(int x, int y)
    {
        return x + y;
    }

    public static Account operator +(Account l, Account r) => new Account(0);
    public static implicit operator int(Account a) => a._a;
}
