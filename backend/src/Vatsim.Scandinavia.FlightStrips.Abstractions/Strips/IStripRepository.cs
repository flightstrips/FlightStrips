namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public interface IStripRepository
{
    Task<bool> UpsertAsync(StripUpsertRequest upsertRequest);

    Task DeleteAsync(string callsign);

    Task<Strip?> GetAsync(string callsign);

    Task SetSequenceAsync(string callsign, int? sequence);
}
