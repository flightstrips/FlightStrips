namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public class UpsertBayRequest
{
    public required string Id { get; set; }

    public bool Default { get; set; }

    public string[] CallsignFilter { get; set; } = Array.Empty<string>();
}
