Imports shop.data
Imports Data2 = shop.data

Namespace Shop.App
    Public Class Runner
        Public Sub Run()
            Dim r As REPO = New repo()
            r.SAVE(1)
            Helper.Assist()
        End Sub
    End Class
End Namespace
