using Microsoft.AspNetCore.Mvc;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
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
        var group = builder.MapGroup("/strips").WithTags("Strips");

        group.MapGet("{session}/{airport}/{callsign}", GetStripAsync)
            .WithOpenApi()
            .WithName("GetStrip")
            .WithSummary("Gets a strip from identifier.")
            .Produces<Strip>();

        group.MapPost("{session}/{airport}/{callsign}", UpsertAsync)
            .WithOpenApi()
            .WithName("UpsertStrip")
            .WithSummary("Upsert strip")
            .WithDescription("Create strip if it does not exist, otherwise update")
            .Produces(StatusCodes.Status204NoContent)
            .Produces(StatusCodes.Status201Created);

        group.MapPost("{session}/{airport}/{callsign}/{sequence:int}", SetSequenceAsync)
            .WithOpenApi().WithName("SetStripSequence")
            .WithSummary("Set the sequence for a strip")
            .Produces(StatusCodes.Status204NoContent);
        
        return group;
    }


    private static async Task<IResult> UpsertAsync([AsParameters] StripId id,
        [FromBody] UpsertStripRequestModel request, [FromServices] IStripService service)
    {
        var upsertRequest = new StripUpsertRequest
        {
            Id = id,
            Destination = request.Destination,
            Origin = request.Origin,
            State = request.State,
            Cleared = request.Cleared
        };
        var created = await service.UpsertStripAsync(upsertRequest);

        return created
            ? Results.CreatedAtRoute("GetStrip", new {id.Session, id.Airport, id.Callsign})
            : Results.NoContent();
    }

    private static async Task<IResult> GetStripAsync([AsParameters] StripId id, [FromServices] IStripService service)
    {
        var strip = await service.GetStripAsync(id);

        return strip is null ? Results.NotFound() : Results.Ok(strip);
    }

    private static async Task<IResult> SetSequenceAsync([AsParameters] StripId id, [FromRoute] int sequence, [FromServices] IStripService service)
    {
        await service.SetSequenceAsync(id, sequence);

        return Results.NoContent();

    }
    
}