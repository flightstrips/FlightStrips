using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class CoordinationService : ICoordinationService
{
    private readonly ICoordinationRepository _repository;
    private readonly IStripRepository _stripRepository;
    private readonly IEventService _eventService;

    public CoordinationService(ICoordinationRepository repository, IStripRepository stripRepository, IEventService eventService)
    {
        _repository = repository;
        _stripRepository = stripRepository;
        _eventService = eventService;
    }

    public Task<Coordination[]> ListForFrequencyAsync(SessionId session, string frequency)
    {
        return _repository.ListForFrequency(session, frequency);
    }

    public Task<Coordination?> GetForCallsignAsync(SessionId session, string callsign) => _repository.GetForCallsignAsync(session, callsign);

    public Task<Coordination?> GetAsync(CoordinationId id) => _repository.GetAsync(id);

    public async Task AcceptAsync(CoordinationId id, string frequency)
    {
        var coordination = await GetAsync(id);

        if (coordination is null)
        {
            throw new InvalidOperationException("Coordination does not exist");
        }

        await _repository.DeleteAsync(id);
        await _stripRepository.SetPositionFrequencyAsync(coordination.StripId, frequency);
        await _eventService.AcceptCoordinationAsync(coordination);
    }

    public async Task RejectAsync(CoordinationId id, string frequency)
    {
        var coordination = await _repository.GetAsync(id);
        if (coordination is null)
        {
            return;
        }

        await _repository.DeleteAsync(id);
        await _eventService.RejectCoordinationAsync(coordination);
    }

    public async Task<int> CreateAsync(Coordination coordination)
    {
        var id = await _repository.CreateAsync(coordination);
        coordination.Id = id;

        await _eventService.StartCoordinationAsync(coordination);

        return id;
    }
}
