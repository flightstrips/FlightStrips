using Vatsim.Scandinavia.FlightStrips.Abstractions.Entities;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;

public interface IStripService
{
    Strip CreateStrip(StripCreateRequest createRequest);
    Strip UpdateStrip(Strip updatedStrip);
    void DeleteStrip(StripId id);
    Task<Strip?> GetStripAsync(StripId stripId);
}