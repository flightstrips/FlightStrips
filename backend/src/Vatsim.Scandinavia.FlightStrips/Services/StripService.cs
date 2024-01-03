using Microsoft.Extensions.Logging;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class StripService : IStripService
{
    private readonly IStripRepository _stripRepository;
    private readonly IBayService _bayService;
    private readonly IEventService _eventService;
    private readonly ILogger<StripService> _logger;

    public StripService(IStripRepository stripRepository, ILogger<StripService> logger, IBayService bayService, IEventService eventService)
    {
        _stripRepository = stripRepository;
        _logger = logger;
        _bayService = bayService;
        _eventService = eventService;
    }

    public async Task<bool> UpsertStripAsync(StripUpsertRequest upsertRequest)
    {
        var strip = await GetStripAsync(upsertRequest.Id);

        if (strip is not null)
        {
            upsertRequest.Bay = strip.Bay;
        }

        if (string.IsNullOrEmpty(upsertRequest.Bay))
        {
            var isDeparture = upsertRequest.Id.Airport.Equals(upsertRequest.Origin, StringComparison.OrdinalIgnoreCase);

            var bay = await _bayService.GetDefault(upsertRequest.Id.Airport, upsertRequest.Id.Callsign, isDeparture);

            if (string.IsNullOrEmpty(bay))
            {
                throw new InvalidOperationException("Must have a bay");

            }

            upsertRequest.Bay = bay;
        }

        var created = await _stripRepository.UpsertAsync(upsertRequest);
        strip = await _stripRepository.GetAsync(upsertRequest.Id);

        if (created)
        {
            await _eventService.StripCreatedAsync(strip!);
        }
        else
        {
            await _eventService.StripUpdatedAsync(strip!);
        }

        return created;
    }

    public async Task DeleteStripAsync(StripId id)
    {
        var strip = await _stripRepository.GetAsync(id);

        if (strip is null)
        {
            return;
        }

        await _stripRepository.DeleteAsync(id);
        await _eventService.StripDeletedAsync(strip);
    }

    public Task<Strip?> GetStripAsync(StripId id)
    {
        return _stripRepository.GetAsync(id);
    }

    public async Task SetSequenceAsync(StripId id, int? sequence)
    {

        _logger.LogInformation("Setting sequence for {Strip} to {Sequence}", id, sequence);

        await _stripRepository.SetSequenceAsync(id, sequence);
        var strip = await _stripRepository.GetAsync(id);
        await _eventService.StripUpdatedAsync(strip!);

    }

    public async Task SetBayAsync(StripId id, string bayName)
    {
        await _stripRepository.SetBayAsync(id, bayName);
        var strip = await _stripRepository.GetAsync(id);
        await _eventService.StripUpdatedAsync(strip!);
    }

    public async Task AssumeAsync(StripId id, string frequency)
    {
        await _stripRepository.SetPositionFrequencyAsync(id, frequency);
        var strip = await _stripRepository.GetAsync(id);
        await _eventService.StripUpdatedAsync(strip!);
    }
}
