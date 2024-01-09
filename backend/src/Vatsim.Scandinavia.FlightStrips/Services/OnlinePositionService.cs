using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class OnlinePositionService : IOnlinePositionService
{
    private readonly IOnlinePositionRepository _repository;
    private readonly IEventService _eventService;

    public OnlinePositionService(IOnlinePositionRepository repository, IEventService eventService)
    {
        _repository = repository;
        _eventService = eventService;
    }

    public async Task CreateAsync(OnlinePositionId id, string frequency)
    {
        await _repository.AddAsync(new OnlinePositionAddRequest(id, frequency));
        await _eventService.ControllerOnlineAsync(new OnlinePosition
        {
            Id = id,
            PrimaryFrequency = frequency
        });
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
    }

    public Task<OnlinePosition[]> ListAsync(string airport, string session) =>
        _repository.ListAsync(airport.ToUpperInvariant(), session.ToUpperInvariant());
}
