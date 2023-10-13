using Microsoft.Extensions.DependencyInjection;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;
using Vatsim.Scandinavia.FlightStrips.Services;

namespace Vatsim.Scandinavia.FlightStrips.Extensions;

public static class FlightStripExtensions
{
    public static IServiceCollection AddFlightStripServices(this IServiceCollection services)
    {
        services.AddSingleton<IStripService, StripService>();
        services.AddSingleton<IBayService, BayService>();

        return services;
    }
    
}