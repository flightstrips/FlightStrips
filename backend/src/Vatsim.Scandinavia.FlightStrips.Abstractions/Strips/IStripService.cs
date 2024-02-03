namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public interface IStripService
{
    Task<(bool created, Strip strip)> UpsertStripAsync(StripUpsertRequest upsertRequest);
    Task DeleteStripAsync(StripId id);
    Task<Strip?> GetStripAsync(StripId id);
    Task SetSequenceAsync(StripId id, int? sequence);
    Task SetBayAsync(StripId id, string bayName);
    Task AssumeAsync(StripId id, string frequency);
    Task<SessionId[]> GetSessionsAsync();
    Task RemoveSessionAsync(SessionId id);
    Task ClearAsync(StripId id, bool isCleared);
}
