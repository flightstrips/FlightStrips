namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public interface IOnlinePositionRepository
{
    Task AddAsync(OnlinePositionAddRequest request);
    Task DeleteAsync(OnlinePositionId id);
    Task<OnlinePosition[]> ListAsync(string airport, string session, bool onlyEuroscopeConnected = false);
    Task<OnlinePosition?> GetAsync(OnlinePositionId id);
    Task<SessionId[]> GetSessionsAsync();
    Task RemoveSessionAsync(SessionId id);
    Task BulkSetSectorAsync(SessionId id, IEnumerable<OnlinePosition> positions);
    Task SetRunwaysAsync(OnlinePositionId id, string? departure, string? arrival);
    Task SetUiOnlineAsync(OnlinePositionId id, bool online);
}
