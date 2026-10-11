' SZENARIO: Kaputter VB-Code darf nicht abstürzen. Gute Deklarationen nach dem Fehler bleiben erhalten.
' ABDECKUNG: Klasse ohne End Class, Sub mit fehlender Klammer, Namespace ohne Namen, gute Funktion danach
Namespace Shop
    Public Class Half
        Public Sub Broken(a As Integer
    End Class
End Namespace
Namespace
Public Function Okay() As Integer
    Return 1
End Function
