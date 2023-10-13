namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public class UpsertBayRequest
{
    public required string Airport { get; set; }
    
    public required string Name { get; set; }
    
    public bool Default { get; set; }

    public string[] CallsignFilter { get; set; } = Array.Empty<string>();
}