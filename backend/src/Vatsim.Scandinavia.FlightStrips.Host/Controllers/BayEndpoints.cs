using System.ComponentModel.DataAnnotations;

using Microsoft.AspNetCore.Mvc;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

public static class BayEndpoints
{
    public static IEndpointRouteBuilder MapBays(this IEndpointRouteBuilder builder)
    {
        var group = builder.MapGroup("/bays").WithTags("Bays");

        group.MapPut("{airport}/{name}", UpsertAsync);

        return group;
    }

    private static async Task<IResult> UpsertAsync([FromRoute] [RegularExpression("^[A-Z]*$")] string airport,
        [FromRoute] [RegularExpression("^[a-z]*$")] string name,
        [FromBody] UpsertBayRequestModel request,
        [FromServices] IBayService service)
    {
        var upsertRequest = new UpsertBayRequest
        {
            Name = name, Airport = airport, CallsignFilter = request.CallsignFilter, Default = request.Default
        };

        await service.UpsertAsync(upsertRequest);
        
        return Results.NoContent();
    }
}
