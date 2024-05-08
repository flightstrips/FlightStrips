using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public static partial class LogMessages
{
    [LoggerMessage(LogLevel.Information, "Cleanup service started.")]
    public static partial void CleanupServiceStarted(this ILogger logger);

    [LoggerMessage(LogLevel.Error, "Error occurred executing cleanup.")]
    public static partial void CleanupServiceErrorDuringProcessing(this ILogger logger, Exception exception);

    [LoggerMessage(LogLevel.Information, "Cleanup service shutting down.")]
    public static partial void CleanupServiceShuttingDown(this ILogger logger);


    [LoggerMessage(LogLevel.Information, "Connection {ConnectionId} subscribed to session {Airport} {Session}")]
    public static partial void ConnectionSubscribedToAirport(this ILogger logger, string connectionId, string airport, string session);

    [LoggerMessage(LogLevel.Information, "Controller {Callsign} subscribed on {Frequency} for session {Airport} {Session}")]
    public static partial void ControllerSubscribed(this ILogger logger, string callsign, string frequency, string airport, string session);

    [LoggerMessage(LogLevel.Information, "Controller unsubscribed on {Frequency} for session {Airport} {Session}")]
    public static partial void ControllerUnsubscribed(this ILogger logger, string frequency, string airport, string session);

    [LoggerMessage(LogLevel.Information, "Connection removed {ConnectionId}")]
    public static partial void ConnectionRemove(this ILogger logger, string connectionId);

    [LoggerMessage(LogLevel.Information, "Removing inactive session {Airport} {Session}")]
    public static partial void RemovingInactiveSession(this ILogger logger, string airport, string session);

    [LoggerMessage(LogLevel.Information, "Sending strip update for {Callsign}. {Airport} {Session}. To group {Group}")]
    public static partial void SendingStripUpdate(this ILogger logger, string callsign, string airport, string session, string group);

    [LoggerMessage(LogLevel.Information, "Got message {MessageType} from EuroScope from client {ClientId}. Message: {Message}")]
    public static partial void GotEuroScopeMessage(this ILogger logger, string clientId, string messageType, string message);

    [LoggerMessage(LogLevel.Information, "Processed message from EuroScope from client {ClientId} in {Time} ms.")]
    public static partial void ProcessedEuroScopeMessage(this ILogger logger, string clientId, double time);

    [LoggerMessage(LogLevel.Information, "EuroScope client '{ClientId}' disconnected.")]
    public static partial void EuroScopeClientDisconnected(this ILogger logger, string clientId);

    [LoggerMessage(LogLevel.Information, "No new master available for session {Session}.")]
    public static partial void NoNewMasterAvailable(this ILogger logger, SessionId session);

    [LoggerMessage(LogLevel.Information, "Removing EuroScope master client {Controller} from session {Session}.")]
    public static partial void RemovingEuroScopeMaster(this ILogger logger, SessionId session, string controller);

    [LoggerMessage(LogLevel.Information, "Setting new EuroScope master client {Controller} for session {Session}.")]
    public static partial void SettingNewEuroScopeMaster(this ILogger logger, SessionId session, string controller);

    [LoggerMessage(LogLevel.Information, "Removing EuroScope client {Controller} from session {Session}.")]
    public static partial void RemovedEuroScopeClient(this ILogger logger, SessionId session, string controller);

    [LoggerMessage(LogLevel.Information, "Added EuroScope client {Controller} to session {Session}.")]
    public static partial void AddedEuroScopeClient(this ILogger logger, SessionId session, string controller);
}
