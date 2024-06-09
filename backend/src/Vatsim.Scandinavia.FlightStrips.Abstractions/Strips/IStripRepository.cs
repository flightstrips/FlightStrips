using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public interface IStripRepository
{
    Task<(bool, Strip)> UpsertAsync(StripUpsertRequest upsertRequest);

    Task<Strip[]> ListAsync(SessionId id);

    Task DeleteAsync(StripId id);

    Task<Strip?> GetAsync(StripId id);

    Task SetSequenceAsync(StripId id, int? sequence);
    Task SetBayAsync(StripId id, string bayName);
    Task SetPositionFrequencyAsync(StripId id, string frequency);
    Task<SessionId[]> GetSessionsAsync();
    Task RemoveSessionAsync(SessionId id);
    Task SetCleared(StripId id, bool isCleared, string bay);
    Task CreateAsync(Strip strip);
    Task UpdateAsync(Strip strip);
    Task<bool> SetStandAsync(StripId id, string stand);

    Task<bool> SetSquawk(StripId id, string squawk);
    Task<bool> SetAssignedSquawkAsync(StripId id, string squawk);
    Task<bool> SetFinalAltitudeAsync(StripId id, int altitude);
    Task<bool> SetClearedAltitudeAsync(StripId id, int altitude);
    Task<bool> SetGroundStateAsync(StripId id, StripState state);
    Task<bool> SetCommunicationTypeAsync(StripId id, CommunicationType communicationType);
    Task SetPositionAsync(StripId id, Position position);
}
