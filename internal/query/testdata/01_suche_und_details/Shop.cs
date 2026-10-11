namespace Shop.Catalog;

/// <summary>Verwaltet Artikel im Katalog.</summary>
public class Catalog
{
    /// <summary>Liefert den Artikel zur Nummer.</summary>
    /// <param name="number">Die Artikelnummer.</param>
    /// <returns>Der Artikel oder null.</returns>
    public Item Find(int number) => null;

    internal void Reindex() { }
}

public class Item
{
    public string Title { get; set; }
}

internal class Cache
{
    public void Find() { }
}
