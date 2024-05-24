using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Host.Models.Runways;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("{airport:required}/{session:required}/runways")]
public class RunwayController(IRunwayService runwayService) : ControllerBase
{
    [HttpGet(Name = "GetRunwayConfiguration")]
    [ProducesResponseType(typeof(RunwayConfigResponseModel), StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> GetRunwayConfigAsync(string airport, string session)
    {
        var config = await runwayService.GetRunwayConfigAsync(new SessionId(airport, session));

        if (config is null)
        {
            return NotFound();
        }

        var model = new RunwayConfigResponseModel
        {
            Position = config.Position,
            Arrival = config.Arrival,
            Departure = config.Departure
        };

        return Ok(model);
    }
}
