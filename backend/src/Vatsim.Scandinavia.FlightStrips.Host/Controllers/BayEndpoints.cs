using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Host.Extensions;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

public static class BayEndpoints
{
    public static IEndpointRouteBuilder MapBays(this IEndpointRouteBuilder builder)
    {
        var group = builder.MapAirportTenantRoutes("bays").WithTags("Bays");

        group.MapPut("{name}", UpsertAsync)
            .WithName("UpsertBay")
            .WithSummary("Create or update bay");
        group.MapGet("", ListAsync)
            .WithName("ListBays")
            .WithSummary("Retrieve bays")
            .Produces<Bay[]>();
        group.MapDelete("{name}", DeleteAsync)
            .WithName("DeleteBay")
            .WithSummary("Delete bay.")
            .Produces(StatusCodes.Status204NoContent);

        return group;
    }

    private static async Task<IResult> UpsertAsync(
        [FromRoute] string name,
        [FromBody] UpsertBayRequestModel request,
        [FromServices] IBayService service)
    {
        var upsertRequest = new UpsertBayRequest
        {
            Id = name, CallsignFilter = request.CallsignFilter, Default = request.Default
        };

        await service.UpsertAsync(upsertRequest);

        return Results.NoContent();
    }

    private static async Task<IResult> DeleteAsync([FromRoute] string name,
        [FromServices] IBayService service)
    {
        await service.DeleteAsync(name);

        return Results.NoContent();
    }

    private static async Task<IResult> ListAsync([FromServices] IBayService service)
    {
        var bays = await service.ListAsync();

        return Results.Ok(bays);
    }
}
