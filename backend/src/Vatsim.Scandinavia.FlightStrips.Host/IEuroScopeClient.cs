using FlightStrips;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public interface IEuroScopeClient
{
    Task<bool> WriteAsync(ServerStreamMessage message);

    string Controller { get; }

    string Frequency { get; }

    int Range { get; }
}
