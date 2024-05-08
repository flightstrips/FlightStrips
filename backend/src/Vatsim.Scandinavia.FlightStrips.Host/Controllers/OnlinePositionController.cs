using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("{airport:required}/{session:required}/online-positions")]
public class OnlinePositionController(IOnlinePositionService onlinePositionService) : ControllerBase
{
    [HttpGet(Name = "ListOnlinePositions")]
    [ProducesResponseType(typeof(OnlinePositionResponseModel[]), StatusCodes.Status200OK)]
    [ProducesResponseType(typeof(ValidationProblemDetails), StatusCodes.Status400BadRequest)]
    public async Task<IActionResult> ListAsync([Airport] string airport, string session)
    {
        var positions = await onlinePositionService.ListAsync(airport, session);
        var models = positions.Select(Map).ToArray();
        return Ok(models);
    }

    private static OnlinePositionResponseModel Map(OnlinePosition position)
    {
        return new OnlinePositionResponseModel { Frequency = position.PrimaryFrequency, Position = position.Id.Position };
    }
}
