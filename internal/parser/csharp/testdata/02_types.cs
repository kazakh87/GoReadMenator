// SZENARIO: Alle Typarten mit Modifikatoren, Generics (Arity im Namen), verschachtelten Typen und Record-Varianten
// ABDECKUNG: class, struct, interface, record, record struct, enum mit Werten, generische Typen mit Arity, verschachtelte Typen, abstract, sealed, static
namespace Shop;

public abstract class Entity { }
internal sealed class Money { }
public struct Point { public int X; public int Y; }
public interface IRepository<T> where T : class { }
public record Order(int Id, string Customer);
public record struct Coord(double Lat, double Lon);
public enum Status { Open, Closed = 5, Archived }
public class Pair<TFirst, TSecond> { }
public static class Strings { }
public class Outer
{
    public class Inner { }
    public enum Kind { A, B }
}
