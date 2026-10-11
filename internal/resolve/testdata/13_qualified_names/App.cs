namespace Shop.App
{
    public class Runner
    {
        public Shop.Models.Order Full;          // vollqualifiziert: Shop.Models.Order
        public Models.Order Relative;            // relativ zu Shop: Shop.Models.Order
        public Archive.Order Other;              // vollqualifiziert: Archive.Order
        public Shop.Models.Box<Archive.Order> Nested;
    }
}
