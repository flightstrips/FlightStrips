using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;
using Vatsim.Scandinavia.FlightStrips.Host.Mappers;
using Vatsim.Scandinavia.FlightStrips.Host.Models;
using CoordinationState = Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations.CoordinationState;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("{airport}/{session}/strips")]
public class StripController : ControllerBase
{
    private readonly IStripService _stripService;
    private readonly ICoordinationService _coordinationService;
    private readonly IBayService _bayService;

    public StripController(IStripService stripService, ICoordinationService coordinationService, IBayService bayService)
    {
        _stripService = stripService;
        _coordinationService = coordinationService;
        _bayService = bayService;
    }

    [HttpGet("{callsign}", Name = "GetStrip")]
    [ProducesResponseType(typeof(StripResponseModel), StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> GetStripAsync([Airport, FromRoute] string airport, [FromRoute] string session,
        [Callsign, FromRoute] string callsign)
    {
        var id = new StripId(airport, session, callsign);
        var strip = await _stripService.GetStripAsync(id);
        if (strip is null)
        {
            return NotFound();
        }

        var model = new StripResponseModel
        {
            Callsign = strip.Id.Callsign,
            Bay = strip.Bay,
            Controller = strip.PositionFrequency,
            Cleared = strip.Cleared,
            Destination = strip.Destination,
            Origin = strip.Origin,
            Sequence = strip.Sequence,
            Alternate = strip.Alternate,
            Capabilities = strip.Capabilities,
            Remarks = strip.Remarks,
            Route = strip.Route,
            Runway = strip.Runway,
            Squawk = strip.Squawk,
            Stand = strip.Stand,
            Tobt = strip.TOBT,
            AircraftCategory = strip.AircraftCategory,
            AircraftType = strip.AircraftType,
            AssignedSquawk = strip.AssignedSquawk,
            CommunicationType = strip.CommunicationType,
            Heading = strip.Heading,
            Sid = strip.Sid,
            Tsat = strip.TSAT,
            ClearedAltitude = strip.ClearedAltitude,
            FinalAltitude = strip.FinalAltitude
        };

        return Ok(model);
    }

    [HttpPost("{callsign}/clear", Name = "ClearStrip")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> ClearAsync([Airport] string airport, string session,
        [Callsign, FromRoute] string callsign, [FromBody] StripClearRequestModel request)
    {
        var id = new StripId(airport, session, callsign);
        var strip = await _stripService.GetStripAsync(id);
        if (strip is null) return NotFound();

        await _stripService.ClearAsync(id, request.IsCleared);

        return NoContent();
    }

    [HttpPost("{callsign}/move", Name = "MoveStrip")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    public async Task<IActionResult> MoveAsync([Airport] string airport, string session,
        [Callsign, FromRoute] string callsign, [FromBody] StripMoveRequestModel request)
    {
        var id = new StripId(airport, session, callsign);
        var strip = await _stripService.GetStripAsync(id);
        if (strip is null) return NotFound();

        var bay = await _bayService.GetAsync(id.Airport, request.Bay.ToUpperInvariant());
        if (bay is null) return NotFound();

        await _stripService.SetBayAsync(id, bay.Name);
        await _stripService.SetSequenceAsync(id, request.Sequence);

        return NoContent();
    }

    [HttpPost("{callsign}/assume", Name = "AssumeStrip")]
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

    [HttpPost("{callsign}/transfer", Name = "TransferStrip")]
    [ProducesResponseType(typeof(CoordinationResponseModel), StatusCodes.Status200OK)]
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

        var model = CoordinationMapper.Map(coordination);

        return CreatedAtAction("Get", "Coordination", new {airport, session, id = coordination.Id}, model);
    }
}
