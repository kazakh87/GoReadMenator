' SZENARIO: Module, Interface mit Methodensignaturen ohne Körper, Structure, Enum mit Werten
' ABDECKUNG: Module mit Sub Main, Interface mit Sub und Property ohne Körper, Structure, Enum mit Wert, Enum ohne Wert
Namespace Shop
    Public Module Program
        Public Sub Main()
            Console.WriteLine("hi")
        End Sub
    End Module

    Public Interface IRepo
        Sub Save(item As Item)
        Function Find(id As Integer) As Item
        Property Count As Integer
    End Interface

    Public Structure Point
        Public X As Integer
        Public Y As Integer
    End Structure

    Public Enum Status
        Open
        Closed = 5
    End Enum
End Namespace
