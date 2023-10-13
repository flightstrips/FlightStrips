using Microsoft.Extensions.Logging;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class StripService : IStripService
{
    private readonly IStripRepository _stripRepository;
    private readonly ILogger<StripService> _logger;

    public StripService(IStripRepository stripRepository, ILogger<StripService> logger)
    {
        _stripRepository = stripRepository;
        _logger = logger;
    }

    public Task<bool> UpsertStripAsync(StripUpsertRequest upsertRequest)
    {
        return _stripRepository.UpsertAsync(upsertRequest);
    }

    public Task DeleteStripAsync(StripId id)
    {
        return _stripRepository.DeleteAsync(id);
    }

    public Task<Strip?> GetStripAsync(StripId stripId)
    {
        return _stripRepository.GetAsync(stripId);
    }

    public Task SetSequenceAsync(StripId stripId, int? sequence)
    {
        _logger.LogInformation("Setting sequence for {Strip} to {Sequence}", stripId, sequence);
        
        return _stripRepository.SetSequenceAsync(stripId, sequence);

    }
}