using System.Diagnostics;
using FlightStrips;
using Grpc.Core;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Masters;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Host.Mappers;
using ActiveRunway = Vatsim.Scandinavia.FlightStrips.Abstractions.Runways.ActiveRunway;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public sealed class EuroScopeHandler(ILogger<EuroScopeHandler> logger, IGRpcMapper mapper, IEuroScopeClients euroScopeClients, IServiceScopeFactory serviceScopeFactory) : IDisposable, IEuroScopeClient
{
    private SessionId _session;
    private string? _controller;
    private string? _frequency;
    private int? _range;

    public string Controller
    {
        get => _controller ?? throw new InvalidOperationException("Controller is not set.");
    }

    public string Frequency
    {
        get => _frequency ?? throw new InvalidOperationException("Frequency is not set.");
    }

    public int Range
    {
        get => _range ?? throw new InvalidOperationException("Range is not set.");
    }

    private IServerStreamWriter<ServerStreamMessage> _responseStream = null!;

    private CancellationTokenSource _cancellationTokenSource = null!;

    public async Task StartAsync(IAsyncStreamReader<ClientStreamMessage> requestStream,
        IServerStreamWriter<ServerStreamMessage> responseStream, CancellationToken cancellationToken)
    {
        _responseStream = responseStream;

        _cancellationTokenSource = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);

        try
        {
            await foreach (var message in requestStream.ReadAllAsync(_cancellationTokenSource.Token))
            {
                logger.GotEuroScopeMessage(message.ClientId, message.MessageCase.ToString(), message.ToString());
                var start = Stopwatch.GetTimestamp();

                await using var scope = serviceScopeFactory.CreateAsyncScope();
                await HandleMessageAsync(message, scope.ServiceProvider);
                var timeSpan = Stopwatch.GetElapsedTime(start);
                logger.ProcessedEuroScopeMessage(message.ClientId, timeSpan.TotalMilliseconds);
            }

        }
        finally
        {
            if (!string.IsNullOrWhiteSpace(_controller))
            {
                await euroScopeClients.RemoveClientAsync(new OnlinePositionId(_session.Airport, _session.Session, _controller));
                await using var scope = serviceScopeFactory.CreateAsyncScope();
                var controllerService = scope.ServiceProvider.GetRequiredService<IOnlinePositionService>();
                await controllerService.DeleteAsync(new OnlinePositionId(_session.Airport, _session.Session, _controller));
                logger.EuroScopeClientDisconnected(_controller);
            }

            _cancellationTokenSource.Dispose();
        }
    }

    public async Task<bool> WriteAsync(ServerStreamMessage message)
    {
        // TODO move somewhere else
        if (message.MessageCase == ServerStreamMessage.MessageOneofCase.SessionInfo && message.SessionInfo.IsMaster &&
            !string.IsNullOrEmpty(_controller))
        {
            await using var scope = serviceScopeFactory.CreateAsyncScope();
            var masterService = scope.ServiceProvider.GetRequiredService<IMasterService>();
            var id = new OnlinePositionId(_session.Airport, _session.Session, _controller);
            masterService.SetMaster(id);

            // update runway configuration
            var onlinePositionService = scope.ServiceProvider.GetRequiredService<IOnlinePositionService>();

            var position = await onlinePositionService.GetAsync(id);

            if (position is {ArrivalRunway: not null, DepartureRunway: not null})
            {
                var runwayService = scope.ServiceProvider.GetRequiredService<IRunwayService>();
                await runwayService.SetRunwaysAsync(_session,
                    new RunwayConfig(position.DepartureRunway, position.ArrivalRunway, position.Id.Position));
            }
        }

        // TODO handle error in the connection forcing us to abort and thus remove the client.
        try
        {
            await _responseStream.WriteAsync(message);
        }
        catch (Exception)
        {
            await _cancellationTokenSource.CancelAsync();
            return false;
        }
        return true;
    }

    private async Task HandleMessageAsync(ClientStreamMessage message, IServiceProvider serviceProvider)
    {
        switch (message.MessageCase)
        {
            case ClientStreamMessage.MessageOneofCase.None:
                break;
            case ClientStreamMessage.MessageOneofCase.ClientInfo:
                {
                    var controllerService = serviceProvider.GetRequiredService<IOnlinePositionService>();
                    _session = new SessionId(message.ClientInfo.Session.Airport,
                        message.ClientInfo.Session.Session_);

                    var alreadyOnline = !string.IsNullOrEmpty(_controller);

                    if (!message.ClientId.Equals(_controller, StringComparison.OrdinalIgnoreCase) && alreadyOnline)
                    {
                        await controllerService.DeleteAsync(new OnlinePositionId(_session.Airport, _session.Session,
                            _controller!));
                    }

                    _controller = message.ClientId;
                    _frequency = message.ClientInfo.Frequency;
                    _range = (int)message.ClientInfo.Range;
                    var id = new OnlinePositionId(_session.Airport, _session.Session, _controller);
                    var runways =
                        message.ClientInfo.AirportInfo.Runways?.Select(x => new ActiveRunway(x.Runway, x.Departure))
                            .ToArray() ?? Array.Empty<ActiveRunway>();
                    await controllerService.CreateAsync(id, _frequency, runways, plugin: true);

                    if (alreadyOnline)
                    {
                        return;
                    }

                    // TODO future send existing strips for when client is maintaining information regarding each flight
                    // and holds TSAT and so on.

                    // Client is now ready for messages.
                    await euroScopeClients.AddClientAsync(id, this);
                    break;
                }
            case ClientStreamMessage.MessageOneofCase.StripData:
                {
                    if (_session == default) return;

                    var stripService = serviceProvider.GetRequiredService<IStripService>();
                    switch (message.StripData.MessageCase)
                    {
                        case StripData.MessageOneofCase.None:
                            break;
                        case StripData.MessageOneofCase.AssignedSquawk:
                            await stripService.SetAssignedSquawkAsync(mapper.MapStripId(message.StripData, _session),
                                message.StripData.AssignedSquawk.Squawk_);
                            break;
                        case StripData.MessageOneofCase.FinalAltitude:
                            await stripService.SetFinalAltitudeAsync(mapper.MapStripId(message.StripData, _session),
                                (int)message.StripData.FinalAltitude.Altitude);
                            break;
                        case StripData.MessageOneofCase.ClearedAltitude:
                            await stripService.SetClearedAltitudeAsync(mapper.MapStripId(message.StripData, _session),
                                (int)message.StripData.ClearedAltitude.Altitude);
                            break;
                        case StripData.MessageOneofCase.CommunicationType:
                            await stripService.SetCommunicationTypeAsync(mapper.MapStripId(message.StripData, _session),
                                mapper.Map(message.StripData.CommunicationType));
                            break;
                        case StripData.MessageOneofCase.GroundState:
                            await stripService.SetGroundStateAsync(mapper.MapStripId(message.StripData, _session),
                                mapper.Map(message.StripData.GroundState.State));
                            break;
                        case StripData.MessageOneofCase.Cleared:
                            await stripService.ClearAsync(
                                new StripId(_session.Airport, _session.Session, message.StripData.Callsign),
                                message.StripData.Cleared.Cleared, Sender.EuroScope);
                            break;
                        case StripData.MessageOneofCase.SetSquawk:
                            await stripService.SetSquawkAsync(mapper.MapStripId(message.StripData, _session),
                                message.StripData.SetSquawk.Squawk_);
                            break;
                        case StripData.MessageOneofCase.Position:
                            await stripService.HandleStripPositionUpdateAsync(mapper.MapPosition(message.StripData,
                                _session));
                            break;
                        case StripData.MessageOneofCase.FullData:
                            await stripService.HandleStripUpdateAsync(mapper.MapFull(message.StripData, _session));
                            break;
                        case StripData.MessageOneofCase.Disconnect:
                            // TODO maybe give a bit of time before removing all data for the strip. It may just be a
                            // temporary disconnect.
                            await stripService.DeleteStripAsync(mapper.MapStripId(message.StripData, _session));
                            break;
                        default:
                            throw new UnreachableException("Missing enum case");
                    }

                    break;
                }
            case ClientStreamMessage.MessageOneofCase.AirportInfo:
                {
                    if (_session == default || string.IsNullOrEmpty(_controller)) return;

                    var id = new OnlinePositionId(_session.Airport, _session.Session, _controller);
                    var runways = message.AirportInfo.Runways.Select(x => new ActiveRunway(x.Runway, x.Departure)).ToArray();

                    var controllerService = serviceProvider.GetRequiredService<IOnlinePositionService>();
                    await controllerService.SetRunwaysAsync(id, runways);
                    break;
                }
            case ClientStreamMessage.MessageOneofCase.ControllerUpdate:
                {
                    if (!message.ControllerUpdate.HasFrequency || message.ControllerUpdate.Frequency.StartsWith("0", StringComparison.OrdinalIgnoreCase)) return;
                    if (!message.ControllerUpdate.Callsign.StartsWith("EKCH", StringComparison.OrdinalIgnoreCase) &&
                        !message.ControllerUpdate.Callsign.StartsWith("EKDK", StringComparison.OrdinalIgnoreCase)) return;

                    var controllerService = serviceProvider.GetRequiredService<IOnlinePositionService>();
                    var id = new OnlinePositionId(_session.Airport, _session.Session,
                        message.ControllerUpdate.Callsign);
                    switch (message.ControllerUpdate.ConnectionStatus)
                    {
                        case ConnectionStatus.Unspecified:
                            break;
                        case ConnectionStatus.Connected:
                            await controllerService.UpsertAsync(id, message.ControllerUpdate.Frequency);
                            break;
                        case ConnectionStatus.Disconnected:
                            await controllerService.DeleteAsync(id);
                            break;
                        default:
                            throw new UnreachableException("Unhandled enum.");
                    }

                    break;
                }
            default:
                throw new UnreachableException("Missing enum case");
        }
    }

    public void Dispose()
    {
        _cancellationTokenSource.Dispose();
    }
}
