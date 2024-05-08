using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Events;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public interface IEuroScopeService
{
    Task HandleFullStripEvent(FullStripEvent stripEvent);
}
