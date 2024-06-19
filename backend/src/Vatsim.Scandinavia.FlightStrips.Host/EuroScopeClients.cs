using System.Collections.Concurrent;
using System.Runtime.InteropServices;
using FlightStrips;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Position = Vatsim.Scandinavia.FlightStrips.Abstractions.Positions.Position;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public sealed class EuroScopeClients(ILoggerFactory loggerFactory) : IEuroScopeClients
{
    private const int Range = 50;
    private readonly ConcurrentDictionary<SessionId, SessionClients> _sessionClients = new();

    public async Task<bool> AddClientAsync(OnlinePositionId id, IEuroScopeClient client)
    {
        var clients = _sessionClients.GetOrAdd(new SessionId(id.Airport, id.Session), CreateSessionClients);
        await clients.AddClientAsync(id.Position, client);

        return true;
    }

    private SessionClients CreateSessionClients(SessionId session)
    {
        return new SessionClients(loggerFactory.CreateLogger<SessionClients>(), session);
    }

    public async Task RemoveClientAsync(OnlinePositionId id)
    {
        var clients = _sessionClients.GetOrAdd(new SessionId(id.Airport, id.Session), CreateSessionClients);
        await clients.RemoveClientAsync(id.Position);
    }

    public Task WriteToControllerClientAsync(SessionId session, string controller, ServerStreamMessage message)
    {
        if (!_sessionClients.TryGetValue(session, out var client))
        {
            return Task.CompletedTask;
        }

        return client.WriteToControllerAsync(controller, message);
    }


    private sealed class SessionClients(ILogger<SessionClients> logger, SessionId sessionId) : IDisposable
    {
        private readonly Dictionary<string, IEuroScopeClient> _clients = new(StringComparer.OrdinalIgnoreCase);
        private readonly SemaphoreSlim _semaphore = new(1, 1);

        // TODO move
        // TODO future: allow APP and CTR to be master when they are the only ones online...
        private static readonly Position[] _preferredMasters =
        [
            Position.EKCH_A_TWR, Position.EKCH_D_TWR, Position.EKCH_C_TWR, Position.EKCH_A_GND, Position.EKCH_D_GND,
            Position.EKCH_DEL
        ];

        private IEuroScopeClient? _master;


        public Task<bool> WriteToMasterAsync(ServerStreamMessage message)
        {
            if (_master is null) return Task.FromResult(false);

            return WriteToClientAsync(_master.Controller, message);

        }

        public Task<bool> WriteToControllerAsync(string controller, ServerStreamMessage message)
        {
            return WriteToClientAsync(controller, message);
        }

        public async Task WriteToAllAsync(ServerStreamMessage message)
        {
            var tasks = _clients.Keys.Select(x => WriteToClientAsync(x, message)).ToArray();

            await Task.WhenAll(tasks);
        }

        private async Task<bool> WriteToClientAsync(string controller, ServerStreamMessage message)
        {
            if (!_clients.TryGetValue(controller, out var client)) return false;

            var result = await client.WriteAsync(message);

            if (!result)
            {
                _clients.Remove(controller);
            }

            return result;
        }

        public async Task AddClientAsync(string controller, IEuroScopeClient client, CancellationToken cancellationToken = default)
        {
            await _semaphore.WaitAsync(cancellationToken);
            try
            {
                if (!_clients.TryAdd(controller, client))
                {
                    return;
                }

                logger.AddedEuroScopeClient(sessionId, controller);

                var newMaster = FindBestMaster();
                if (newMaster is null || newMaster == _master) return;

                await UpdateMasterAsync(newMaster);
            }
            finally
            {
                _semaphore.Release();
            }
        }


        public async Task RemoveClientAsync(string controller, CancellationToken cancellationToken = default)
        {
            await _semaphore.WaitAsync(cancellationToken);
            try
            {
                _clients.Remove(controller, out var client);
                logger.RemovedEuroScopeClient(sessionId, controller);
                if (_master != client)
                {
                    return;
                }


                var newMaster = FindBestMaster();
                if (newMaster is null)
                {
                    logger.NoNewMasterAvailable(sessionId);
                    return;
                }

                await UpdateMasterAsync(newMaster);

            }
            finally
            {
                _semaphore.Release();
            }
        }

        private IEuroScopeClient? FindBestMaster()
        {
            var clients = _clients.ToDictionary(x => x.Value.Frequency, x => x.Value);

            foreach (var preferredMaster in _preferredMasters)
            {
                if (clients.TryGetValue(preferredMaster.Frequency, out var value)) return value;
            }

            return _clients.Values.FirstOrDefault();
        }


        private async Task UpdateMasterAsync(IEuroScopeClient newMaster)
        {
            if (_master is not null)
            {
                logger.RemovingEuroScopeMaster(sessionId, _master.Controller);
                await _master.WriteAsync(new ServerStreamMessage
                {
                    SessionInfo = new SessionInfo {IsMaster = false, RelevantRange = Range}
                });
            }

            logger.SettingNewEuroScopeMaster(sessionId, newMaster.Controller);
            var result = await newMaster.WriteAsync(new ServerStreamMessage()
            {
                SessionInfo = new SessionInfo {IsMaster = true, RelevantRange = Range}
            });

            if (result)
            {
                _master = newMaster;
            }
        }

        public void Dispose()
        {
            _semaphore.Dispose();
        }
    }

}
