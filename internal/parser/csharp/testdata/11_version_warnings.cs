// SZENARIO: Konstrukte neuerer C#-Versionen, die der Parser nur teilweise erfasst. Er warnt, statt still falsch zu arbeiten.
// ABDECKUNG: primärer Konstruktor (C# 12), Positionsparameter eines Records (C# 9), file-lokaler Typ (C# 11), Kommentar mit Muster bleibt ohne Warnung
namespace Shop;

// public class FakePrimary(int a) { }
public class Point(int x, int y)
{
    public int Sum() => x + y;
}

public record Order(int Id, string Customer);

file class Hidden
{
    public void Run() { }
}

public class Plain
{
    public void Go() { }
}
