using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("api/{airport:required}/bays")]
public class BayController(IBayService bayService) : ControllerBase
{
    [HttpGet(Name = "ListBays")]
    [Produces(typeof(Bay[]))]
    public async Task<IActionResult> ListAsync([Airport] string airport)
    {
        var bays = await bayService.ListAsync(airport);
        return Ok(bays);
    }

}

