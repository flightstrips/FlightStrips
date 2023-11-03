using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class OnlinePositionService : IOnlinePositionService
{
    private readonly IOnlinePositionRepository _repository;

    public OnlinePositionService(IOnlinePositionRepository repository)
    {
        _repository = repository;
    }

    public Task CreateAsync(string controllerId, string frequency) =>
        _repository.AddAsync(new OnlinePositionAddRequest(controllerId, frequency));

    public Task DeleteAsync(string controllerId) => _repository.DeleteAsync(controllerId);

    public Task<OnlinePosition[]> ListAsync() => _repository.ListAsync();
}
