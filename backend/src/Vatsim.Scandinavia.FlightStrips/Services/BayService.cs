using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class BayService : IBayService
{
    private readonly IBayRepository _bayRepository;

    public BayService(IBayRepository bayRepository)
    {
        _bayRepository = bayRepository;
    }

    public Task<bool> UpsertAsync(UpsertBayRequest request)
    {
        return _bayRepository.UpsertAsync(request);
    }

    public Task DeleteAsync(BayId id)
    {
        return _bayRepository.DeleteAsync(id);
    }

    public Task<Bay?> GetAsync(BayId id)
    {
        return _bayRepository.GetAsync(id);
    }

    public async Task<BayId?> GetDefault(string airport, string callsign)
    {
        callsign = callsign.ToUpperInvariant();

        var company = callsign.Trim()[..2];

        var defaultBays = (await _bayRepository.ListAsync(new ListBaysRequest(airport, Default: true))).ToArray();

        if (defaultBays.Length == 0)
        {
            return null;
        }

        var bay = defaultBays.FirstOrDefault(x => x.CallsignFilter.Any() && x.CallsignFilter.Contains(company)) ??
                  defaultBays.FirstOrDefault(x => !x.CallsignFilter.Any());

        if (bay is null)
        {
            return null;
        }

        return new BayId(bay.Airport, bay.Name);
    }
}
