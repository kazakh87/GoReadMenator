// SZENARIO: Texte, die wie Code aussehen, dürfen keine Symbole erzeugen. Präprozessor-Zweige mit gleichen Deklarationen werden zusammengeführt.
// ABDECKUNG: Zeilenkommentar, Blockkommentar, normaler String mit Escapes, verbatim-String mit "", Rohstring """, Zeichenliteral mit Klammer, #if/#else-Duplikate
namespace Shop;

// public class FakeInLineComment { }
/* public class FakeInBlockComment { } */
public class Literals
{
    string a = "class FakeInString { void M() {} }";
    string b = @"class FakeVerbatim ""quoted"" { }";
    string c = """
        class FakeRaw { }
        """;
    char d = '{';
    string e = "escaped \" quote class FakeEscape { }";
#if NET8
    public void Run() { }
#else
    public void Run() { }
#endif
}
