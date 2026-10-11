' SZENARIO: Attribute an Typ und Member mit Argumenten, XML-Dokumentation mit Summary, Parametern und Rückgabe
' ABDECKUNG: Attribut am Typ, mehrere Attribute in einer Liste, Attribut mit Argument am Member, Doku mit summary, param, returns, remarks
Namespace Shop
    <Serializable>
    <Obsolete("alt"), Route("api/orders")>
    Public Class Orders
        ''' <summary>Startet <c>Verarbeitung</c> &amp; Prüfung.</summary>
        ''' <param name="id">Die Nummer.</param>
        ''' <returns>Das Ergebnis.</returns>
        <Required>
        Public Function Start(id As Integer) As Boolean
            Return True
        End Function
    End Class
End Namespace
