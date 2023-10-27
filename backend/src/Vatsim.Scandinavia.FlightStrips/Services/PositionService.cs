using Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class PositionService : IPositionService
{
    private readonly IPositionRepository _repository;

    public PositionService(IPositionRepository repository)
    {
        _repository = repository;
    }

    public Task UpsertAsync(UpsertPositionRequest request) => _repository.UpsertAsync(request);

    public Task DeleteAsync(string frequency) => _repository.DeleteAsync(frequency);

    public Task<Position?> GetAsync(string frequency) => _repository.GetAsync(frequency);

    public Task<Position[]> ListAsync() => _repository.ListAsync();
}
