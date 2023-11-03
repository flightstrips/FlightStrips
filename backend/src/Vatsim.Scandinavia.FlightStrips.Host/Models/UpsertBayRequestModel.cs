namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class UpsertBayRequestModel
{
    public bool Default { get; set; }

    public string[] CallsignFilter { get; set; } = Array.Empty<string>();
}