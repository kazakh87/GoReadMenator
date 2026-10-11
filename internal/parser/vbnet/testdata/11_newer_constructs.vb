' SZENARIO: Neuere VB-Konstrukte: async und await, Tupel als Rückgabetyp, Iterator mit Yield, Interpolation, If als Ausdruck und NameOf
' ABDECKUNG: Async Function mit Task(Of T), Await-Aufruf, benannte Tupel als Rückgabetyp (Namen sind keine Typen), Iterator/Yield, $-Interpolation, If(a, b, c) ohne Aufruf, NameOf ohne Aufruf
Namespace App
    Public Class Svc
        Public Async Function Fetch(id As Integer) As Task(Of String)
            Return Await Load(id)
        End Function

        Public Function Pair() As (Left As Integer, Right As String)
            Return (1, "a")
        End Function

        Public Sub Show()
            Dim msg = $"Hallo {Name}"
            Dim x = If(msg.Length > 2, 1, 0)
            Console.WriteLine(NameOf(msg))
        End Sub

        Public Iterator Function Items() As IEnumerable(Of Integer)
            Yield 1
        End Function
    End Class
End Namespace
