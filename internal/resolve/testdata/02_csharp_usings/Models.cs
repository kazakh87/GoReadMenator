namespace Shop.Models
{
    public class Customer { }
    public static class Limits
    {
        public const int Max = 10;
        public static int Clamp(int x) => x;
    }
}

namespace Shop.Hidden
{
    public class Secret { }
}
