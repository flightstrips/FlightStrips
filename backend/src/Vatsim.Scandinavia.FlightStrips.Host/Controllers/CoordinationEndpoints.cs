using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Extensions;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

public static class CoordinationEndpoints
{
    public static IEndpointRouteBuilder MapCoordination(this IEndpointRouteBuilder builder)
    {
        var group = builder.MapAirportAndSessionTenantRoutes("coordination").WithTags("Coordination");

        group.MapGet("{frequency}", ListForFrequencyAsync).WithName("ListCoordination")
            .WithDescription("List coordination ongoing for frequency").Produces<Coordination[]>()
            .ProducesValidationProblem();

        group.MapPost("{id:int}/accept", AcceptAsync)
            .WithName("AcceptCoordination")
            .WithDescription("Accept coordination")
            .Produces(StatusCodes.Status204NoContent)
            .Produces(StatusCodes.Status404NotFound)
            .ProducesValidationProblem();

        group.MapPost("{id:int}/reject", RejectAsync)
            .WithName("RejectCoordination")
            .WithDescription("Reject coordination")
            .Produces(StatusCodes.Status204NoContent)
            .Produces(StatusCodes.Status404NotFound)
            .ProducesValidationProblem();

        return group;
    }

    private static async Task<IResult> ListForFrequencyAsync([FromRoute, Frequency] string frequency,
        [FromServices] ICoordinationService service)
    {
        var coordinations = await service.ListForFrequencyAsync(frequency);

        return Results.Ok(coordinations);
    }

    private static async Task<IResult> AcceptAsync([FromRoute] int id,
        [FromBody] AcceptCoordinationRequestModel request,
        [FromServices] ICoordinationService service)
    {
        var coordination = await service.GetAsync(id);

        if (coordination is null)
        {
            return Results.NotFound();
        }

        if (!coordination.ToFrequency.Equals(request.Frequency, StringComparison.OrdinalIgnoreCase))
        {
            return Results.BadRequest("Can't accept coordination which is not address to you.");
        }

        await service.AcceptAsync(id, request.Frequency);

        return Results.NoContent();
    }

    private static async Task<IResult> RejectAsync([FromRoute] int id,
        [FromBody] RejectCoordinationRequestModel request,
        [FromServices] ICoordinationService service)
    {
        var coordination = await service.GetAsync(id);

        if (coordination is null)
        {
            return Results.NotFound();
        }

        if (!coordination.ToFrequency.Equals(request.Frequency, StringComparison.OrdinalIgnoreCase))
        {
            return Results.BadRequest("Can't reject coordination which is not address to you.");
        }

        await service.RejectAsync(id, request.Frequency);

        return Results.NoContent();
    }
}
