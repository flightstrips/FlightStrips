using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public interface IOnlinePositionService
{
    Task CreateAsync(OnlinePositionId id, string frequency, ActiveRunway[] runways, bool plugin = false, bool ui = false);
    Task UpsertAsync(OnlinePositionId id, string? frequency = null, ActiveRunway[]? runways = null, bool? ui = null);
    Task DeleteAsync(OnlinePositionId id);
    Task<OnlinePosition?> GetAsync(OnlinePositionId id);
    Task<OnlinePosition[]> ListAsync(string airport, string session, bool onlyEuroscopeConnected = false);
    Task<SessionId[]> GetSessionsAsync();
    Task RemoveSessionAsync(SessionId id);

    Task UpdateSectorsAsync(SessionId id);
    Task SetRunwaysAsync(OnlinePositionId id, ActiveRunway[] runways);

    Task SetUiOnlineAsync(OnlinePositionId id, bool online);
}

