using Microsoft.Extensions.DependencyInjection;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public static class EfServiceCollectionExtensions
{
    public static IServiceCollection AddEfCore(this IServiceCollection services)
    {
        services.AddScoped<IStripRepository, EfStripRepository>();
        services.AddScoped<IOnlinePositionRepository, EfOnlinePositionRepository>();
        services.AddScoped<ICoordinationRepository, EfCoordinationRepository>();
        services.AddScoped<IRunwayRepository, EfRunwayRepository>();

        return services;
    }
}
