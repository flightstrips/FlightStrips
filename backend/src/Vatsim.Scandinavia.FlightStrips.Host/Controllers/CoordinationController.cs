using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Mappers;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("{airport:required}/{session:required}/coordination")]
[Tags("Coordination")]
public class CoordinationController : ControllerBase
{
    private readonly ICoordinationService _coordinationService;

    public CoordinationController(ICoordinationService coordinationService)
    {
        _coordinationService = coordinationService;
    }

    [HttpGet("{frequency}")]
    [ProducesResponseType(typeof(CoordinationResponseModel[]), StatusCodes.Status200OK)]
    [ProducesResponseType(typeof(ValidationProblemDetails), StatusCodes.Status400BadRequest)]
    public async Task<IActionResult> ListForFrequencyAsync([Airport] string airport, string session,
        [FromRoute, Frequency] string frequency)
    {
        var sessionId = new SessionId(airport, session);
        var coordinations = await _coordinationService.ListForFrequencyAsync(sessionId, frequency);
        var models = coordinations.Select(CoordinationMapper.Map).ToArray();
        return Ok(models);
    }

    [HttpPost("{id:int}/accept")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    [ProducesResponseType(typeof(ValidationProblemDetails), StatusCodes.Status400BadRequest)]
    public async Task<IActionResult> AcceptAsync([Airport] string airport, string session, int id, [FromBody] AcceptCoordinationRequestModel request)
    {
        var coordinationId = new CoordinationId(airport, session, id);
        var coordination = await _coordinationService.GetAsync(coordinationId);

        if (coordination is null)
        {
            return NotFound();
        }

        if (!coordination.ToFrequency.Equals(request.Frequency, StringComparison.OrdinalIgnoreCase))
        {
            return BadRequest("Can't accept coordination which is not addressed to you.");
        }

        await _coordinationService.AcceptAsync(coordinationId, request.Frequency);
        return NoContent();
    }

    [HttpPost("{id:int}/reject")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    [ProducesResponseType(typeof(ValidationProblemDetails), StatusCodes.Status400BadRequest)]
    public async Task<IActionResult> RejectAsync([Airport] string airport, string session, int id, [FromBody] RejectCoordinationRequestModel request)
    {
        var coordinationId = new CoordinationId(airport, session, id);
        var coordination = await _coordinationService.GetAsync(coordinationId);

        if (coordination is null)
        {
            return NotFound();
        }

        if (!coordination.ToFrequency.Equals(request.Frequency, StringComparison.OrdinalIgnoreCase))
        {
            return BadRequest("Can't reject coordination which is not addressed to you.");
        }

        await _coordinationService.RejectAsync(coordinationId, request.Frequency);
        return NoContent();
    }
}
