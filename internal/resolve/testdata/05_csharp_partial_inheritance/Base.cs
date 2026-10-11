namespace Domain;

[Serializable]
public abstract class Entity { }

public interface IStore { }

public partial class Repo : Entity, IStore
{
    public Item Find(Key k) => null;
}

public class Item { }
public class Key { }
