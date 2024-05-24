using Microsoft.Extensions.DependencyInjection;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Masters;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Stands;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Services;

namespace Vatsim.Scandinavia.FlightStrips.Extensions;

public static class FlightStripExtensions
{
    public static IServiceCollection AddFlightStripServices(this IServiceCollection services)
    {
        services.AddScoped<IStripService, StripService>();
        services.AddScoped<IBayService, BayService>();
        services.AddScoped<IOnlinePositionService, OnlinePositionService>();
        services.AddScoped<ICoordinationService, CoordinationService>();
        services.AddScoped<IOwnerService, OwnerService>();
        services.AddScoped<IRunwayService, RunwayService>();

        services.AddSingleton<IMasterService, MasterService>();
        services.AddSingleton<IStandService, StandService>();

        return services;
    }

}
