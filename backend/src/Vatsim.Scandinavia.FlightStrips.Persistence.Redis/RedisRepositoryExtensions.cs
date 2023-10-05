using Microsoft.Extensions.DependencyInjection;

using StackExchange.Redis;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.Redis;

public static class RedisRepositoryExtensions
{
    public static IServiceCollection AddRedisStorage(this IServiceCollection services, RedisOptions options)
    {
        services.AddSingleton<IConnectionMultiplexer>(_ =>
            ConnectionMultiplexer.Connect(options.GetConnectionString()));

        services.AddSingleton<IDatabase>(provider =>
            provider.GetRequiredService<IConnectionMultiplexer>().GetDatabase(0));

        services.AddSingleton<IStripRepository, RedisStripRepository>();
        
        return services;
    }

}