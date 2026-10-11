Imports Web.Models

Namespace Web.Services
    Public Class OrderService
        Public Function Find(id As Integer) As Order
            Return Nothing
        End Function

        Public Sub Save(o As Order)
        End Sub

        Public Sub Audit()
            Dim o As New Order()
            SAVE(o)
        End Sub
    End Class
End Namespace
