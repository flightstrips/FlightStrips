namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public class Bay
{
    public required string Name { get; set; }

    public bool Default { get; set; }

    public string[] CallsignFilter { get; set; } = Array.Empty<string>();
}
