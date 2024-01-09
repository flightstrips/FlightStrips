using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;
using Vatsim.Scandinavia.FlightStrips.Host.Models;
using CoordinationState = Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations.CoordinationState;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("{airport:required}/{session:required}/strips")]
public class StripController : ControllerBase
{
    private readonly IStripService _stripService;
    private readonly ICoordinationService _coordinationService;

    public StripController(IStripService stripService, ICoordinationService coordinationService)
    {
        _stripService = stripService;
        _coordinationService = coordinationService;
    }

    [HttpGet("{callsign}")]
    [ProducesResponseType(typeof(StripResponseModel), StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> GetStripAsync([Airport] string airport, string session,
        [Callsign, FromRoute] string callsign)
    {
        var id = new StripId(airport, session, callsign);
        var strip = await _stripService.GetStripAsync(id);
        if (strip is null)
        {
            return NotFound();
        }

        var model = new StripResponseModel()
        {
            Callsign = strip.Id.Callsign,
            Bay = strip.Bay,
            Controller = strip.PositionFrequency,
            Cleared = strip.Cleared,
            Destination = strip.Destination,
            Origin = strip.Origin,
            Sequence = strip.Sequence
        };

        return Ok(model);
    }

    [HttpPost("{callsign}")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status201Created)]
    public async Task<IActionResult> UpsertAsync([Airport] string airport, string session,
        [Callsign, FromRoute] string callsign, [FromBody] UpsertStripRequestModel request)
    {
        var upsertRequest = new StripUpsertRequest
        {
            Id = new StripId(airport, session, callsign),
            Destination = request.Destination,
            Origin = request.Origin,
            State = request.State,
            Cleared = request.Cleared
        };
        var created = await _stripService.UpsertStripAsync(upsertRequest);

        return created
            ? CreatedAtRoute("GetStrip", new { airport, session, callsign })
            : NoContent();
    }

    [HttpPost("{callsign}/move")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> MoveAsync([Airport] string airport, string session,
        [Callsign, FromRoute] string callsign, [FromBody] StripMoveRequestModel request)
    {
        var id = new StripId(airport, session, callsign);
        var strip = await _stripService.GetStripAsync(id);
        if (strip is null) return NotFound();

        await _stripService.SetBayAsync(id, request.Bay);
        await _stripService.SetSequenceAsync(id, request.Sequence);

        return NoContent();
    }

    [HttpPost("{callsign}/assume")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status400BadRequest)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> AssumeAsync([Airport] string airport, string session,
        [Callsign, FromRoute] string callsign, [FromBody] StripAssumeRequestModel request)
    {
        var id = new StripId(airport, session, callsign);
        var strip = await _stripService.GetStripAsync(id);
        if (strip is null) return NotFound();

        if (!request.Force && !string.IsNullOrEmpty(strip.PositionFrequency))
            return BadRequest("Can't assume strip when assumed by another controller.");

        await _stripService.AssumeAsync(id, request.Frequency);
        return NoContent();
    }

    [HttpPost("{callsign}/transfer")]
    [ProducesResponseType(typeof(Coordination), StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status400BadRequest)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> TransferAsync([Airport] string airport, string session,
        [Callsign, FromRoute] string callsign, [FromBody] StripTransferRequestModel request)
    {
        var id = new StripId(airport, session, callsign);
        var strip = await _stripService.GetStripAsync(id);
        if (strip is null) return NotFound();

        if (!strip.PositionFrequency?.Equals(request.CurrentFrequency, StringComparison.OrdinalIgnoreCase) ?? false)
            return BadRequest("Can't transfer a strip you don't own.");

        var sessionId = new SessionId(airport, session);
        var coordination = await _coordinationService.GetForCallsignAsync(sessionId, callsign);
        if (coordination is not null)
            return BadRequest("A request is already started for callsign.");

        coordination = new Coordination
        {
            StripId = id,
            FromFrequency = request.CurrentFrequency,
            ToFrequency = request.ToFrequency,
            State = CoordinationState.Transfer
        };

        var coordinationId = await _coordinationService.CreateAsync(coordination);
        coordination.Id = coordinationId;

        return Ok(coordination);
    }
}
