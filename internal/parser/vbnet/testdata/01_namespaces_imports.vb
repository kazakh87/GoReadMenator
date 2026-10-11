' SZENARIO: Namensräume (verschachtelt), Imports mit und ohne Alias, mehrere Namensräume in einer Datei
' ABDECKUNG: Namespace-Block, verschachtelter Namespace, Imports, Imports mit Alias, Namensraum ohne Typen
Imports System
Imports System.Collections.Generic
Imports Fs = System.IO

Namespace Shop
    Namespace Core
        Public Class Marker
        End Class
    End Namespace
End Namespace

Namespace Shop.Empty
End Namespace
