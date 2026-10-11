// SZENARIO: Kaputter Quelltext darf nicht abstürzen. Es entstehen Diagnosen, die übrigen Deklarationen bleiben erhalten.
// ABDECKUNG: fehlende Klammer am Ende, unvollständige Deklaration, Parameterliste nicht geschlossen, gute Deklaration danach
namespace Shop;

public class Half
{
    public void Ok() { }
    public int Broken(int a, string
}

public class After { }
