namespace Shop.Core;

public class Order
{
    public class Line { }
    public void Save(int id) { }
    public void Run()
    {
        Save(1);          // EXACT über Scope: Member der eigenen Klasse
        var l = new Line(); // EXACT über Scope: verschachtelter Typ
        Helper.Assist();  // Qualifier: Helper ist im selben Namensraum
    }
}

public static class Helper
{
    public static void Assist() { }
}
