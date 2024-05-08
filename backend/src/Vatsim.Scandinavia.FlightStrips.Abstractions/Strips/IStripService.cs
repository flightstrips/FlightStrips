using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Events;

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
    Task HandleStripUpdateAsync(FullStripEvent stripEvent);
    Task HandleStripPositionUpdateAsync(PositionEvent positionEvent);
    Task SetSquawkAsync(StripId id, string squawk);
    Task SetAssignedSquawkAsync(StripId id, string squawk);
    Task SetFinalAltitudeAsync(StripId id, int altitude);
    Task SetClearedAltitudeAsync(StripId id, int altitude);
    Task SetCommunicationTypeAsync(StripId id, CommunicationType communicationType);
    Task SetGroundStateAsync(StripId id, StripState state);
}
