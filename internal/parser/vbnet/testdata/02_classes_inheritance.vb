' SZENARIO: Klassen mit Basisklasse und Interface-Implementierung, Modifikatoren, Partial
' ABDECKUNG: Class, Inherits, Implements (mehrere), Public/Private/Friend, MustInherit, Partial Class, verschachtelte Klasse
Namespace Shop
    Public MustInherit Class Entity
    End Class

    Public Class Order
        Inherits Entity
        Implements IPriced, IDisposable

        Public Class Line
        End Class
    End Class

    Partial Friend Class Ledger
    End Class

    Partial Friend Class Ledger
        Public Sub Post()
        End Sub
    End Class
End Namespace
