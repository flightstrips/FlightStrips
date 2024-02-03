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

    [LoggerMessage(LogLevel.Information, "Sending strip update for {Callsign} state {State}. {Airport} {Session}. To group {Group}")]
    public static partial void SendingStripUpdate(this ILogger logger, string callsign, string airport, string session, StripState state, string group);

}
