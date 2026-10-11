// SZENARIO: UTF-8-BOM am Dateianfang und Windows-Zeilenenden (CRLF). Positionen müssen trotzdem stimmen.
// ABDECKUNG: BOM, CRLF-Zeilenenden, Umlaute in Bezeichnern und Kommentaren
namespace Shop;
public class Größe
{
    public int Breite { get; set; }
}
