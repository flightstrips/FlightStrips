using Microsoft.EntityFrameworkCore.Diagnostics;
using Microsoft.Extensions.DependencyInjection;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public static class EfServiceCollectionExtensions
{
    public static IServiceCollection AddEfCore(this IServiceCollection services)
    {
        services.AddScoped<IBayRepository, EfBayRepository>();
        services.AddScoped<IStripRepository, EfStripRepository>();
        services.AddScoped<IPositionRepository, EfPositionRepository>();

        return services;
    }
}
