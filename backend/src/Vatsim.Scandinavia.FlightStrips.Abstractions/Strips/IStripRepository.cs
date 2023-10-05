namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public interface IStripRepository
{
    Task CreateAsync(StripCreateRequest createRequest);

    Task DeleteAsync(StripId stripId);

    Task<Strip?> GetAsync(StripId stripId);

    Task SetSequenceAsync(StripId stripId, int? sequence);
}