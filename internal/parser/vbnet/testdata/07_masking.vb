' SZENARIO: Texte und Kommentare dürfen keine Symbole erzeugen. Anführungszeichen werden verdoppelt, Zeilenfortsetzungen und Doppelpunkt-Anweisungen werden richtig gelesen.
' ABDECKUNG: Kommentar mit Apostroph, REM-Kommentar, String mit "" Escape, Zeilenfortsetzung mit _, Doppelpunkt trennt Anweisungen, Bezeichner in eckigen Klammern
Namespace Shop
    ' Public Class FakeInComment
    REM Public Class FakeRem
    Public Class Texts
        Public Const Quoted As String = "Public Class Fake ""inside"" string"
        Public Sub Go(x As Integer, _
                      y As Integer)
            Dim a = 1 : Dim b = 2
            Dim c = Compute(a, _
                            b)
            Dim [Class] = 3
        End Sub
        Private Function Compute(a As Integer, b As Integer) As Integer
            Return a + b
        End Function
    End Class
End Namespace
