using Microsoft.AspNetCore.Http.Metadata;
using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.Routing.Patterns;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
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
        
            
        
        return group;
    }


    private static IResult UpsertAsync([AsParameters] StripId id, [FromServices] IStripService service)
    {
        return Results.NoContent();


    }

    private static async Task<IResult> GetStripAsync([AsParameters] StripId id, [FromServices] IStripService service)
    {
        var strip = await service.GetStripAsync(id);

        return strip is null ? Results.NotFound() : Results.Ok(strip);
    }
    
}