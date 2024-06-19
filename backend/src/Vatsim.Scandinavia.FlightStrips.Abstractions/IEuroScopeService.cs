using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public interface IEuroScopeService
{
    Task SetClearedAsync(StripId id, string controller, bool isCleared);
}
