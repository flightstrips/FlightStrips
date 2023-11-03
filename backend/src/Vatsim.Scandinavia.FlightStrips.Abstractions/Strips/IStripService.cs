namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public interface IStripService
{
    Task<bool> UpsertStripAsync(StripUpsertRequest upsertRequest);
    Task DeleteStripAsync(string callsign);
    Task<Strip?> GetStripAsync(string callsign);
    Task SetSequenceAsync(string callsign, int? sequence);
    Task SetBayAsync(string callsign, string bayName);
    Task AssumeAsync(string callsign, string frequency);
}
