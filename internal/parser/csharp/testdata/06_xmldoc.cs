// SZENARIO: XML-Dokumentation: Summary, Remarks, Returns, Parameter, Typparameter, see/seealso mit cref, Entities und mehrzeilige Texte
// ABDECKUNG: summary, remarks, returns, param, typeparam, see cref, seealso cref, XML-Entities, mehrzeilige Doku, Doku nur an Member
namespace Shop;

/// <summary>
/// Verarbeitet <see cref="Order"/> Bestellungen &amp; Zahlungen.
/// </summary>
/// <remarks>Nur für <c>Shop</c> gedacht.</remarks>
public class Processor<T>
{
    /// <summary>Startet die Verarbeitung.</summary>
    /// <param name="order">Die <see cref="Order"/>, die verarbeitet wird.</param>
    /// <typeparam name="T">Der Ergebnistyp.</typeparam>
    /// <returns>Das Ergebnis, siehe <seealso cref="Processor{T}.Stop()"/>.</returns>
    public T Start(Order order) => default;

    /// <summary>
    /// Stoppt
    /// in mehreren Zeilen.
    /// </summary>
    public void Stop() { }
}
