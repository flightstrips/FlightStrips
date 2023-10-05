using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class StripService : IStripService
{
    public Strip CreateStrip(StripCreateRequest createRequest)
    {
        throw new NotImplementedException();
    }

    public Strip UpdateStrip(Strip updatedStrip)
    {
        throw new NotImplementedException();
    }

    public void DeleteStrip(StripId id)
    {
        throw new NotImplementedException();
    }

    public Task<Strip?> GetStripAsync(StripId stripId)
    {
        throw new NotImplementedException();
    }

    public Task SetSequence(string callsign, int sequence)
    {
        // take all strips after sequence, and before current value and increment by one
        
        // set strip == callsign, equal sequence.

        return Task.CompletedTask;

    }
}