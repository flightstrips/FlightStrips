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
        var strip = await GetStripAsync(upsertRequest.Callsign);

        if (strip is not null)
        {
            upsertRequest.Bay = strip.Bay;
        }

        if (string.IsNullOrEmpty(upsertRequest.Bay))
        {
            var bay = await _bayService.GetDefault(upsertRequest.Callsign);

            if (string.IsNullOrEmpty(bay))
            {
                throw new InvalidOperationException("Must have a bay");

            }

            upsertRequest.Bay = bay;
        }

        var created = await _stripRepository.UpsertAsync(upsertRequest);
        strip = await _stripRepository.GetAsync(upsertRequest.Callsign);

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

    public async Task DeleteStripAsync(string callsign)
    {
        var strip = await _stripRepository.GetAsync(callsign);

        if (strip is null)
        {
            return;
        }

        await _stripRepository.DeleteAsync(callsign);
        await _eventService.StripDeletedAsync(strip);
    }

    public Task<Strip?> GetStripAsync(string callsign)
    {
        return _stripRepository.GetAsync(callsign);
    }

    public async Task SetSequenceAsync(string callsign, int? sequence)
    {

        _logger.LogInformation("Setting sequence for {Strip} to {Sequence}", callsign, sequence);

        await _stripRepository.SetSequenceAsync(callsign, sequence);
        var strip = await _stripRepository.GetAsync(callsign);
        await _eventService.StripUpdatedAsync(strip!);

    }

    public async Task SetBayAsync(string callsign, string bayName)
    {
        await _stripRepository.SetBayAsync(callsign, bayName);
        var strip = await _stripRepository.GetAsync(callsign);
        await _eventService.StripUpdatedAsync(strip!);
    }

    public async Task AssumeAsync(string callsign, string frequency)
    {
        await _stripRepository.SetPositionFrequencyAsync(callsign, frequency);
        var strip = await _stripRepository.GetAsync(callsign);
        await _eventService.StripUpdatedAsync(strip!);
    }
}
