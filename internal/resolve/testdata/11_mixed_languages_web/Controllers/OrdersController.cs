using Web.Models;
using Web.Services;

namespace Web.Controllers;

public class OrdersController
{
    private OrderService svc = new OrderService();

    public Order Get(int id)
    {
        return svc.Find(id);   // richtige Schreibweise aus VB: wird aufgelöst
    }

    public void Post(Order o)
    {
        svc.Save(o);           // richtige Schreibweise: wird aufgelöst
        svc.save(o);           // falsche Schreibweise in C#: bleibt offen
    }
}
