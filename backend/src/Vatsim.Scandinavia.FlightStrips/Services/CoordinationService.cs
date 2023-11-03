using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class CoordinationService : ICoordinationService
{
    private readonly ICoordinationRepository _repository;
    private readonly IStripRepository _stripRepository;

    public CoordinationService(ICoordinationRepository repository, IStripRepository stripRepository)
    {
        _repository = repository;
        _stripRepository = stripRepository;
    }

    public Task<Coordination[]> ListForFrequencyAsync(string frequency)
    {
        return _repository.ListForFrequency(frequency);
    }

    public Task<Coordination?> GetForCallsignAsync(string callsign) => _repository.GetForCallsignAsync(callsign);

    public Task<Coordination?> GetAsync(int id) => _repository.GetAsync(id);

    public async Task AcceptAsync(int id, string frequency)
    {
        var coordination = await GetAsync(id);

        if (coordination is null)
        {
            throw new InvalidOperationException("Coordination does not exist");
        }

        await _repository.DeleteAsync(id);
        await _stripRepository.SetPositionFrequencyAsync(coordination.Callsign, frequency);

    }

    public Task RejectAsync(int id, string frequency)
    {
        return _repository.DeleteAsync(id);
    }

    public Task<int> CreateAsync(Coordination coordination)
    {
        return _repository.CreateAsync(coordination);
    }
}
