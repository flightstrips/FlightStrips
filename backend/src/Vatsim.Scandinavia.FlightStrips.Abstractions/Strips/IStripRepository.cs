namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public interface IStripRepository
{
    Task<(bool, Strip)> UpsertAsync(StripUpsertRequest upsertRequest);

    Task DeleteAsync(StripId id);

    Task<Strip?> GetAsync(StripId id);

    Task SetSequenceAsync(StripId id, int? sequence);
    Task SetBayAsync(StripId id, string bayName);
    Task SetPositionFrequencyAsync(StripId id, string frequency);
    Task<SessionId[]> GetSessionsAsync();
    Task RemoveSessionAsync(SessionId id);
    Task SetCleared(StripId id, bool isCleared, string bay);
}
