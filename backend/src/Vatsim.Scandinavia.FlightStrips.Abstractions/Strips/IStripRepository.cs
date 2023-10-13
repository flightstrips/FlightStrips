namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public interface IStripRepository
{
    Task<bool> UpsertAsync(StripUpsertRequest upsertRequest);

    Task DeleteAsync(StripId stripId);

    Task<Strip?> GetAsync(StripId stripId);

    Task SetSequenceAsync(StripId stripId, int? sequence);
}