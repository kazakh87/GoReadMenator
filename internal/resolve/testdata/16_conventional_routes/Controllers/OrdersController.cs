public class OrdersController : Controller
{
    public IActionResult Details(int id) => View();
    public IActionResult ListAll() => View();
    private void Helper() { }
}

[Route("api/legacy")]
public class LegacyController : Controller
{
    public IActionResult Items() => View();
}
