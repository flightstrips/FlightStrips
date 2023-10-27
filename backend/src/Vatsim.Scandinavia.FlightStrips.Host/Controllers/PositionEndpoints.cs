using System.ComponentModel.DataAnnotations;
using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;
using Vatsim.Scandinavia.FlightStrips.Host.Extensions;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

public static class PositionEndpoints
{
    public static IEndpointRouteBuilder MapPositions(this IEndpointRouteBuilder builder)
    {
        var group = builder.MapAirportTenantRoutes("positions").WithTags("Positions");

        group.MapPut("{frequency}", UpsertAsync)
            .WithName("UpsertPosition")
            .WithSummary("Create or update position")
            .Produces(StatusCodes.Status204NoContent)
            .ProducesValidationProblem();

        group.MapGet("", ListAsync)
            .WithName("ListPositions")
            .WithSummary("List positions")
            .Produces<Position[]>();

        group.MapDelete("{frequency}", DeleteAsync)
            .WithName("DeletePosition")
            .WithSummary("Delete position")
            .Produces(StatusCodes.Status204NoContent)
            .ProducesValidationProblem();

        return group;
    }

    private static async Task<IResult> UpsertAsync([RegularExpression(@"^\d{3}\.\d{3}$")][FromRoute] string frequency,
        [FromBody] UpsertPositionRequestModel model, [FromServices] IPositionService service)
    {
        var request = new UpsertPositionRequest(frequency, model.Name);

        await service.UpsertAsync(request);

        return Results.NoContent();
    }

    private static async Task<IResult> ListAsync([FromServices] IPositionService service)
    {
        var positions = await service.ListAsync();

        return Results.Ok(positions);
    }

    private static async Task<IResult> DeleteAsync([AsParameters] Test frequency,
        [FromServices] IPositionService service)
    {
        await service.DeleteAsync(frequency.Frequency);

        return Results.NoContent();
    }

    private class Test
    {
        [Required] [FromRoute] [Frequency] public string Frequency { get; set; } = string.Empty;

    }
}
