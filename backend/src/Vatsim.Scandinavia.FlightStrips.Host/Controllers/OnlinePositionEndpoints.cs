using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Host.Extensions;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

public static class OnlinePositionEndpoints
{
    public static IEndpointRouteBuilder MapOnlinePositions(this IEndpointRouteBuilder builder)
    {
        var group = builder.MapAirportAndSessionTenantRoutes("online-positions").WithTags("Online Positions");

        group.MapPost("{id}", CreateAsync).WithName("CreateOnlinePosition").Produces(StatusCodes.Status204NoContent)
            .ProducesValidationProblem();

        group.MapDelete("{id}", DeleteAsync).WithName("DeleteOnlinePosition").Produces(StatusCodes.Status204NoContent)
            .ProducesValidationProblem();

        group.MapGet("", ListAsync).WithName("ListOnlinePositions").ProducesValidationProblem()
            .Produces<OnlinePosition[]>();

        return group;
    }

    private static async Task<IResult> CreateAsync([FromRoute] string id, [FromBody] OnlinePositionCreateRequestModel request, [FromServices] IOnlinePositionService service)
    {
        await service.CreateAsync(id, request.Frequency);

        return Results.NoContent();
    }

    private static async Task<IResult> ListAsync([FromServices] IOnlinePositionService service)
    {
        var positions = await service.ListAsync();

        return Results.Ok(positions);
    }

    private static async Task<IResult> DeleteAsync([FromRoute] string id, [FromServices] IOnlinePositionService service)
    {
        await service.DeleteAsync(id);

        return Results.NoContent();
    }
}
