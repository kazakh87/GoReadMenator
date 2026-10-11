using Microsoft.AspNetCore.Mvc;

namespace Web.Controllers;

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

    [HttpGet("/api/shared/item")]
    public IActionResult ItemA() => Ok();
}

public class ArchiveController : ControllerBase
{
    [HttpGet("/api/shared/item")]
    public IActionResult ItemB() => Ok();
}
