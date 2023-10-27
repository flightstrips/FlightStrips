using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
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

            group.MapPost("{callsign}/{sequence:int}", SetSequenceAsync)
                .WithOpenApi().WithName("SetStripSequence")
                .WithSummary("Set the sequence for a strip")
                .Produces(StatusCodes.Status204NoContent)
                .ProducesProblem(StatusCodes.Status404NotFound)
            .ProducesValidationProblem();

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

    private static async Task<IResult> SetSequenceAsync([Callsign, FromRoute] string callsign,
        [FromRoute] int sequence, [FromServices] IStripService service)
    {
        var strip = await service.GetStripAsync(callsign);

        if (strip is null)
        {
            return Results.NotFound();
        }

        await service.SetSequenceAsync(callsign, sequence);

        return Results.NoContent();
    }
}
