// SZENARIO: Basisklassen und Interfaces (Erkennung über das I-Präfix), mehrere Attribute, Attribute mit Ziel-Präfix und Attribute am Member
// ABDECKUNG: extends, implements, I-Präfix-Heuristik, Basisliste mit Generics, Attribute am Typ, Attribute mit Argumenten, Attribut am Member, assembly-Attribut (ohne Ziel-Symbol, daher kein Verweis), Basis object wird übersprungen
[assembly: System.Runtime.InteropServices.ComVisible(false)]

namespace Shop;

[Serializable]
[Obsolete("alt"), Route("api/orders", Name = "orders")]
public class OrderService : BaseService, IOrderService, IDisposable
{
    [Required]
    public string Id { get; set; }
}

public interface IOrderService : IHandler<Order>, IDisposable { }
public class Plain : object { }
