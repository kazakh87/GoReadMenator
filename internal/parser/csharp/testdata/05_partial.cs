// SZENARIO: partial-Klassen und partial-Member in einer Datei
// ABDECKUNG: partial class, partial method, Rolle partial am Fundort
namespace Shop;

public partial class Ledger
{
    partial void OnLoaded();
}

public partial class Ledger
{
    public void Post() { }
}
