using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Events;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class EuroScopeService : IEuroScopeService
{
    public Task HandleFullStripEvent(FullStripEvent stripEvent)
    {
        return Task.CompletedTask;
    }

}
