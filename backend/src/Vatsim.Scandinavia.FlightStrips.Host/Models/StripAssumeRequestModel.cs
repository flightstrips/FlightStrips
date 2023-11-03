namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class StripAssumeRequestModel
{
    public required string Frequency { get; set; }

    public bool Force { get; set; }
}
