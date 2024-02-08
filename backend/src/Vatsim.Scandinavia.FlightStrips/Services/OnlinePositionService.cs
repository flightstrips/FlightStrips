using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class OnlinePositionService : IOnlinePositionService
{
    private readonly IOnlinePositionRepository _repository;
    private readonly IEventService _eventService;
    private readonly IRunwayRepository _runwayRepository;
    private readonly IOwnerService _ownerService;

    public OnlinePositionService(IOnlinePositionRepository repository, IEventService eventService, IRunwayRepository runwayRepository, IOwnerService ownerService)
    {
        _repository = repository;
        _eventService = eventService;
        _runwayRepository = runwayRepository;
        _ownerService = ownerService;
    }

    public async Task CreateAsync(OnlinePositionId id, string frequency)
    {
        await _repository.AddAsync(new OnlinePositionAddRequest(id, frequency));
        await _eventService.ControllerOnlineAsync(new OnlinePosition
        {
            Id = id,
            PrimaryFrequency = frequency
        });
        await UpdateSectorsAsync(new SessionId(id.Airport, id.Session));
    }

    public async Task DeleteAsync(OnlinePositionId id)
    {
        var position = await _repository.GetAsync(id);
        if (position is null)
        {
            return;
        }
        await _repository.DeleteAsync(id);
        await _eventService.ControllerOfflineAsync(position);
        await UpdateSectorsAsync(new SessionId(id.Airport, id.Session));
    }

    public Task<OnlinePosition[]> ListAsync(string airport, string session) =>
        _repository.ListAsync(airport.ToUpperInvariant(), session.ToUpperInvariant());

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
