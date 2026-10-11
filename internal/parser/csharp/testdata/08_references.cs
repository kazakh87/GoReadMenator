// SZENARIO: Verwendungen im Körper: Aufrufe mit unterschiedlicher Argumentzahl, Konstruktoraufrufe, Generic-Aufrufe, Schreibzugriffe, Member-Zugriff über Feld, Typverweise in Signaturen
// ABDECKUNG: call mit arity, construct, generischer Aufruf, write, this.x =, type-ref in Parameter und Rückgabe, Attribut-Verweis, Qualifier-Aufruf
namespace Shop;

public class Client
{
    private Repo _repo;
    private int _count;

    public Order Load(int id, Filter filter)
    {
        var o = new Order(id, "x");
        _count = 1;
        this._count = 2;
        _repo.Save(o);
        Helper<int>(3);
        Log(id, "a", true);
        return o;
    }

    private void Log(int a, string b, bool c) { }
    private void Log(int a) { }
    private static T Helper<T>(T value) => value;
}
