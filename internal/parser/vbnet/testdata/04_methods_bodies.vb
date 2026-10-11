' SZENARIO: Verwendungen in Methodenkörpern: Aufrufe mit Argumentzahl, Erzeugung mit New, Aufruf über Objekt, Zuweisung, verschachtelte If- und For-Blöcke
' ABDECKUNG: Sub, Function mit Rückgabe, Aufruf mit 0/1/3 Argumenten, New-Ausdruck, Qualifier-Aufruf, Zuweisung, Me.Feld = , If-Block, For-Block, Return
Namespace Shop
    Public Class Client
        Private _repo As Repo

        Public Function Load(id As Integer, filter As Filter) As Order
            Dim o = New Order()
            _repo.Save(o)
            Helper.Assist(id)
            Me._count = 2
            If id > 0 Then
                For i As Integer = 1 To 3
                    Log(i, "a", True)
                Next
            End If
            Return o
        End Function

        Private Sub Log(a As Integer, b As String, c As Boolean)
        End Sub

        Private Sub Log(a As Integer)
        End Sub

        Public Sub Run()
            Log(1)
            Log()
        End Sub
    End Class
End Namespace
