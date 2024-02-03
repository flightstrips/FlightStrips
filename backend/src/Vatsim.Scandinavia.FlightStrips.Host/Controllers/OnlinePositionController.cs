using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("{airport:required}/{session:required}/online-positions")]
public class OnlinePositionController(IOnlinePositionService onlinePositionService) : ControllerBase
{
    [HttpPost("{id}", Name = "CreateOnlinePosition")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(typeof(ValidationProblemDetails), StatusCodes.Status400BadRequest)]
    public async Task<IActionResult> CreateAsync([Airport] string airport, string session, string id,
        [FromBody] OnlinePositionCreateRequestModel request)
    {
        var positionId = new OnlinePositionId(airport, session, id);
        await onlinePositionService.CreateAsync(positionId, request.Frequency);
        return NoContent();
    }

    [HttpGet(Name = "ListOnlinePositions")]
    [ProducesResponseType(typeof(OnlinePositionResponseModel[]), StatusCodes.Status200OK)]
    [ProducesResponseType(typeof(ValidationProblemDetails), StatusCodes.Status400BadRequest)]
    public async Task<IActionResult> ListAsync([Airport] string airport, string session)
    {
        var positions = await onlinePositionService.ListAsync(airport, session);
        var models = positions.Select(Map).ToArray();
        return Ok(models);
    }

    [HttpDelete("{id}", Name = "RemoveOnlinePosition")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(typeof(ValidationProblemDetails), StatusCodes.Status400BadRequest)]
    public async Task<IActionResult> DeleteAsync([Airport] string airport, string session, string id)
    {
        var positionId = new OnlinePositionId(airport, session, id);
        await onlinePositionService.DeleteAsync(positionId);
        return NoContent();
    }

    private static OnlinePositionResponseModel Map(OnlinePosition position)
    {
        return new OnlinePositionResponseModel { Frequency = position.PrimaryFrequency, Position = position.Id.Position };
    }
}
