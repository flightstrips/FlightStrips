using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Extensions;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

// ReSharper disable RouteTemplates.RouteParameterIsNotPassedToMethod

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

public static class StripEndpoints
{
    /// <summary>
    /// Add the strip endpoints.
    /// </summary>
    /// <returns>Return the endpoint builder for the endpoint group.</returns>
    public static IEndpointRouteBuilder MapStrips(this IEndpointRouteBuilder builder)
    {
        var group = builder.MapAirportAndSessionTenantRoutes("strips").WithTags("Strips");

        group.MapGet("{callsign}", GetStripAsync)
            .WithName("GetStrip")
            .WithSummary("Gets a strip from identifier.")
            .Produces<Strip>()
            .ProducesValidationProblem();

        group.MapPost("{callsign}", UpsertAsync)
            .WithName("UpsertStrip")
            .WithSummary("Upsert strip")
            .WithDescription("Create strip if it does not exist, otherwise update")
            .Produces(StatusCodes.Status204NoContent)
            .Produces(StatusCodes.Status201Created)
            .ProducesValidationProblem();

        group.MapPost("{callsign}/move", MoveAsync)
            .WithName("MoveStrip")
            .WithSummary("Move strip to bay and set sequence")
            .Produces(StatusCodes.Status204NoContent)
            .ProducesValidationProblem();

        group.MapPost("{callsign}/assume", AssumeAsync)
            .WithName("AssumeStrip")
            .WithDescription("Assume a strip.")
            .ProducesValidationProblem()
            .Produces(StatusCodes.Status404NotFound)
            .Produces(StatusCodes.Status204NoContent);

        group.MapPost("{callsign}/transfer", TransferAsync)
            .WithName("TransferStrip")
            .WithDescription("Transfer a strip")
            .ProducesValidationProblem()
            .Produces(StatusCodes.Status404NotFound)
            .Produces<Coordination>();

        return group;
    }


    private static async Task<IResult> UpsertAsync([Callsign, FromRoute] string callsign,
        [FromBody] UpsertStripRequestModel request, [FromServices] IStripService service,
        [FromServices] ITenantService tenantService)
    {
        var upsertRequest = new StripUpsertRequest
        {
            Callsign = callsign,
            Destination = request.Destination,
            Origin = request.Origin,
            State = request.State,
            Cleared = request.Cleared
        };
        var created = await service.UpsertStripAsync(upsertRequest);

        return created
            ? Results.CreatedAtRoute("GetStrip", new {tenantService.Airport, tenantService.Session, callsign})
            : Results.NoContent();
    }

    private static async Task<IResult> GetStripAsync([Callsign, FromRoute] string callsign,
        [FromServices] IStripService service)
    {
        var strip = await service.GetStripAsync(callsign);

        return strip is null ? Results.NotFound() : Results.Ok(strip);
    }

    private static async Task<IResult> MoveAsync([Callsign, FromRoute] string callsign,
        [FromBody] StripMoveRequestModel requestModel, [FromServices] IStripService service)
    {
        var strip = await service.GetStripAsync(callsign);

        if (strip is null)
        {
            return Results.NotFound();
        }

        await service.SetBayAsync(callsign, requestModel.Bay);
        await service.SetSequenceAsync(callsign, requestModel.Sequence);

        return Results.NoContent();
    }

    private static async Task<IResult> AssumeAsync([Callsign, FromRoute] string callsign,
        [FromBody] StripAssumeRequestModel request, [FromServices] IStripService service)
    {
        var strip = await service.GetStripAsync(callsign);

        if (strip is null)
        {
            return Results.NotFound();
        }

        if (!request.Force && !string.IsNullOrEmpty(strip.PositionFrequency))
        {
            return Results.BadRequest("Can't assume strip when assumed by another controller.");
        }

        await service.AssumeAsync(callsign, request.Frequency);
        return Results.NoContent();

    }

    private static async Task<IResult> TransferAsync([Callsign, FromRoute] string callsign,
        [FromBody] StripTransferRequestModel request, [FromServices] ICoordinationService coordinationService,
        [FromServices] IStripService stripService)
    {
        var strip = await stripService.GetStripAsync(callsign);

        if (strip is null)
        {
            return Results.NotFound();
        }

        if (!strip.PositionFrequency?.Equals(request.CurrentFrequency, StringComparison.OrdinalIgnoreCase) ?? false)
        {
            return Results.BadRequest("Can't transfer a strip you don't own.");
        }

        var coordination = await coordinationService.GetForCallsignAsync(callsign);

        if (coordination is not null)
        {
            return Results.BadRequest("A request is already started for callsign.");
        }

        coordination = new Coordination
        {
            Callsign = callsign,
            FromFrequency = request.CurrentFrequency,
            ToFrequency = request.ToFrequency,
            State = CoordinationState.Transfer
        };

        var id = await coordinationService.CreateAsync(coordination);

        coordination.Id = id;

        return Results.Ok(coordination);
    }

}
