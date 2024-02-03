using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class CleanupService(IServiceProvider serviceProvider, ILogger<CleanupService> logger) : BackgroundService
{
    private readonly Dictionary<SessionId, DateTime> _sessions = new();
    private static readonly TimeSpan Wait = TimeSpan.FromSeconds(30);
    private static readonly TimeSpan Timeout = TimeSpan.FromMinutes(5);

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.CleanupServiceStarted();

        await BackgroundProcessing(stoppingToken);
    }

    private async Task BackgroundProcessing(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await using var scope = serviceProvider.CreateAsyncScope();
                await CleanupAsync(scope.ServiceProvider, stoppingToken);
            }
            catch (Exception ex)
            {
                logger.CleanupServiceErrorDuringProcessing(ex);
            }

            try
            {
                await Task.Delay(Wait, stoppingToken);
            }
            catch (OperationCanceledException)
            {
                // ignore
            }
        }

        logger.CleanupServiceShuttingDown();
    }

    private async Task CleanupAsync(IServiceProvider provider, CancellationToken stoppingToken)
    {
        var onlinePositionService = provider.GetRequiredService<IOnlinePositionService>();
        var stripService = provider.GetRequiredService<IStripService>();

        var now = DateTime.UtcNow;
        var stripSessions = await stripService.GetSessionsAsync();

        foreach (var stripSession in stripSessions)
        {
            _sessions.TryAdd(stripSession, now);
        }

        var controllerSessions = await onlinePositionService.GetSessionsAsync();

        foreach (var controllerSession in controllerSessions)
        {
            if (!_sessions.TryAdd(controllerSession, now))
            {
                _sessions[controllerSession] = now;
            }
        }

        var expired = _sessions.Where(x => x.Value.Add(Timeout) <= now).ToArray();
        foreach (var (session, _) in expired)
        {
            logger.RemovingInactiveSession(session.Airport, session.Session);
            _sessions.Remove(session);
            await stripService.RemoveSessionAsync(session);
        }
    }


}
