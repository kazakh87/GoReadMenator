' SZENARIO: Felder (mehrere Deklaratoren, mit Initialisierer), Konstanten, Auto-Properties, Properties mit Get und Set, Events
' ABDECKUNG: Dim und Feld ohne Dim, Const, Feld mit Komma-Liste, Auto-Property, Property mit Get/Set, Event mit Typ, Shared
Namespace Shop
    Public Class Account
        Private _a As Integer = 1, _b As Integer
        Public Const MaxSize As Integer = 10
        Private Shared count As Long
        Public Property Name As String
        Public Property Balance As Decimal
            Get
                Return _a
            End Get
            Set(value As Decimal)
                _a = value
            End Set
        End Property
        Public Event Changed As EventHandler
    End Class
End Namespace
