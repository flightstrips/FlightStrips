using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Masters;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class OnlinePositionService : IOnlinePositionService
{
    private readonly IOnlinePositionRepository _repository;
    private readonly IEventService _eventService;
    private readonly IRunwayRepository _runwayRepository;
    private readonly IOwnerService _ownerService;
    private readonly IMasterService _masterService;
    private readonly IRunwayService _runwayService;

    public OnlinePositionService(IOnlinePositionRepository repository, IEventService eventService,
        IRunwayRepository runwayRepository, IOwnerService ownerService, IMasterService masterService,
        IRunwayService runwayService)
    {
        _repository = repository;
        _eventService = eventService;
        _runwayRepository = runwayRepository;
        _ownerService = ownerService;
        _masterService = masterService;
        _runwayService = runwayService;
    }

    public async Task CreateAsync(OnlinePositionId id, string frequency, ActiveRunway[] runways, bool plugin = false, bool ui = false)
    {
        var (arrival, departure) = RunwayHelper.GetRunways(runways);
        await _repository.AddAsync(new OnlinePositionAddRequest(id, frequency, plugin, ui, departure, arrival));
        await _eventService.ControllerOnlineAsync(new OnlinePosition
        {
            Id = id,
            PrimaryFrequency = frequency
        });
        await UpdateSectorsAsync(new SessionId(id.Airport, id.Session));
    }


    public async Task SetRunwaysAsync(OnlinePositionId id, ActiveRunway[] runways)
    {
        var (arrival, departure) = RunwayHelper.GetRunways(runways);

        /*
        var position = await onlinePositionService.GetAsync(id);

        if (position is null || string.IsNullOrEmpty(position.DepartureRunway) ||
            string.IsNullOrEmpty(position.ArrivalRunway))
        {
            return true;
        }

        await runwayService.SetRunwaysAsync(sessionId,
            new RunwayConfig(position.DepartureRunway, position.ArrivalRunway, position.Id.Position));
        */

        await _repository.SetRunwaysAsync(id, departure, arrival);
        if (!_masterService.IsMaster(id) || string.IsNullOrEmpty(arrival) || string.IsNullOrEmpty(departure))
        {
            return;
        }

        var sessionId = new SessionId(id.Airport, id.Session);
        await _runwayService.SetRunwaysAsync(sessionId, new RunwayConfig(departure, arrival, id.Position));
        await UpdateSectorsAsync(sessionId);
    }

    public Task SetUiOnlineAsync(OnlinePositionId id, bool online) => _repository.SetUiOnlineAsync(id, online);

    public async Task UpsertAsync(OnlinePositionId id, string? frequency = null, ActiveRunway[]? runways = null, bool? ui = null)
    {
        if (frequency is null && runways is null && ui is null) return;

        var position = await _repository.GetAsync(id);

        if (position is null)
        {
            await _repository.AddAsync(new OnlinePositionAddRequest(id, frequency ?? "", false, ui ?? false, null, null));
        }

    }

    public async Task DeleteAsync(OnlinePositionId id)
    {
        var position = await _repository.GetAsync(id);

        if (position is null)
        {
            return;
        }

        var sessionId = new SessionId(id.Airport, id.Session);
        await _repository.DeleteAsync(id);
        await _eventService.ControllerOfflineAsync(position);
        if (_masterService.IsMaster(id))
        {
            // TODO: were do we set the runways again if another master can be selected.
            await _runwayService.DeleteRunwaysAsync(sessionId);
        }
        await UpdateSectorsAsync(sessionId);
    }

    public Task<OnlinePosition?> GetAsync(OnlinePositionId id) => _repository.GetAsync(id);

    public Task<OnlinePosition[]> ListAsync(string airport, string session, bool onlyEuroscopeConnected = false) =>
        _repository.ListAsync(airport.ToUpperInvariant(), session.ToUpperInvariant(), onlyEuroscopeConnected);

    public Task<SessionId[]> GetSessionsAsync() => _repository.GetSessionsAsync();

    public Task RemoveSessionAsync(SessionId id) => _repository.RemoveSessionAsync(id);

    public async Task UpdateSectorsAsync(SessionId id)
    {
        var config = await _runwayRepository.GetRunwayConfig(id);
        var positions = await _repository.ListAsync(id.Airport, id.Session);
        if (positions.Length == 0) return;

        var newPositions = _ownerService.GetOwners(id, config, positions);

        await _repository.BulkSetSectorAsync(id, newPositions);
        await _eventService.SendControllerSectorsAsync(id, await _repository.ListAsync(id.Airport, id.Session));
    }
}
