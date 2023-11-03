using Microsoft.Extensions.Logging;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class StripService : IStripService
{
    private readonly IStripRepository _stripRepository;
    private readonly IBayService _bayService;
    private readonly ILogger<StripService> _logger;

    public StripService(IStripRepository stripRepository, ILogger<StripService> logger, IBayService bayService)
    {
        _stripRepository = stripRepository;
        _logger = logger;
        _bayService = bayService;
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

        return await _stripRepository.UpsertAsync(upsertRequest);
    }

    public Task DeleteStripAsync(string callsign)
    {
        return _stripRepository.DeleteAsync(callsign);
    }

    public Task<Strip?> GetStripAsync(string callsign)
    {
        return _stripRepository.GetAsync(callsign);
    }

    public Task SetSequenceAsync(string callsign, int? sequence)
    {
        _logger.LogInformation("Setting sequence for {Strip} to {Sequence}", callsign, sequence);

        return _stripRepository.SetSequenceAsync(callsign, sequence);

    }

    public Task SetBayAsync(string callsign, string bayName)
    {
        return _stripRepository.SetBayAsync(callsign, bayName);
    }

    public Task AssumeAsync(string callsign, string frequency)
    {
        return _stripRepository.SetPositionFrequencyAsync(callsign, frequency);
    }
}
