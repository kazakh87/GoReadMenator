// SZENARIO: ASP.NET-Controller mit HTTP-Attributen und Präfix [controller], absolute Routen, Minimal-API-Aufrufe mit Parameter
// ABDECKUNG: Route auf Klasse mit [controller], HttpGet ohne und mit Parameter, HttpPost, absoluter Pfad mit führendem /, Route mit Constraint {id:int}, MapGet mit Literal, Kommentar mit Route bleibt ohne Verweis
namespace Web.Controllers;

// [Route("api/fake")]
[Route("api/[controller]")]
public class OrdersController : ControllerBase
{
    [HttpGet]
    public IActionResult List() => Ok();

    [HttpGet("{id:int}")]
    public IActionResult Get(int id) => Ok();

    [HttpPost]
    public IActionResult Create() => Ok();
}

public class ReportsController : ControllerBase
{
    [HttpGet("/api/reports/summary")]
    public IActionResult Summary() => Ok();
}

public static class Routes
{
    public static void Map(WebApplication app)
    {
        app.MapGet("/api/health", () => "ok");
    }
}
