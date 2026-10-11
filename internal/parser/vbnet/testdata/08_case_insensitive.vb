' SZENARIO: VB ist nicht case-sensitiv. Schlüsselwörter und Namen dürfen in beliebiger Schreibweise auftauchen, die Symbol-ID bleibt die Schreibweise der Deklaration.
' ABDECKUNG: gemischte Groß- und Kleinschreibung bei Schlüsselwörtern, Aufruf mit anderer Schreibweise als Deklaration, END in Großbuchstaben
NAMESPACE Shop
    PUBLIC CLASS Order
        PUBLIC SUB Save(id AS INTEGER)
        END sub
    END CLASS
    Public Class Client
        Sub Run()
            order.SAVE(1)
            Dim x As INTEGER = 2
        End sub
    End Class
End namespace
