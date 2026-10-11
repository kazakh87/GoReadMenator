using Shop.Models;
using static Shop.Models.Limits;
using Alias = Shop.Models.Customer;

namespace Shop.Services
{
    public class Service
    {
        Customer c;          // RESOLVED über using
        Alias a;             // RESOLVED über Alias
        int m = Clamp(3);    // RESOLVED über using static
        Secret s;            // Shop.Hidden ist nicht importiert: nicht EXACT
    }
}
