using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;

public interface IStripService
{
    Task<bool> UpsertStripAsync(StripUpsertRequest upsertRequest);
    Task DeleteStripAsync(StripId id);
    Task<Strip?> GetStripAsync(StripId stripId);
    Task SetSequenceAsync(StripId stripId, int? sequence);
}