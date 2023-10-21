using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

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
        request.CallsignFilter = request.CallsignFilter.Select(x => x.ToUpperInvariant()).ToArray();
        return _bayRepository.UpsertAsync(request);
    }

    public Task DeleteAsync(string name)
    {
        return _bayRepository.DeleteAsync(name);
    }

    public Task<Bay?> GetAsync(string name)
    {
        return _bayRepository.GetAsync(name);
    }

    public Task<Bay[]> ListAsync()
    {
        return _bayRepository.ListAsync(new ListBaysRequest(Default: null));
    }

    public async Task<string?> GetDefault(string callsign)
    {
        callsign = callsign.ToUpperInvariant();

        var company = callsign.Trim()[..3];

        var defaultBays = (await _bayRepository.ListAsync(new ListBaysRequest(Default: true))).ToArray();

        if (defaultBays.Length == 0)
        {
            return null;
        }

        var bay = defaultBays.FirstOrDefault(x => x.CallsignFilter.Any() && x.CallsignFilter.Contains(company)) ??
                  defaultBays.FirstOrDefault(x => !x.CallsignFilter.Any());

        return bay?.Name;
    }
}
