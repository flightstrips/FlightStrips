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

    public async Task CreateAsync(string controllerId, string frequency)
    {
        await _repository.AddAsync(new OnlinePositionAddRequest(controllerId, frequency));
        await _eventService.ControllerOnlineAsync(new OnlinePosition
        {
            PositionId = controllerId, PrimaryFrequency = frequency
        });
    }

    public async Task DeleteAsync(string controllerId)
    {
        var position = await _repository.GetAsync(controllerId);
        if (position is null)
        {
            return;
        }
        await _repository.DeleteAsync(controllerId);
        await _eventService.ControllerOfflineAsync(position);
    }

    public Task<OnlinePosition[]> ListAsync() => _repository.ListAsync();
}
